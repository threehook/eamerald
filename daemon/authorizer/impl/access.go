package impl

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/threehook/eamerald/internal/runtime"

	"github.com/open-policy-agent/opa/v1/rego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// Field names of the AuthZEN information model, and of the decision object a
// policy returns.
const (
	typeField     = "type"
	idField       = "id"
	nameField     = "name"
	decisionField = "decision"
	contextField  = "context"
)

// subjectJWTProperty carries a bearer token on an AuthZEN subject. AuthZEN
// does not standardise how a token reaches the PDP; eamerald reads it from
// this property so that a subject can arrive unresolved, exactly as
// IDENTITY_TYPE_JWT does on the Topaz APIs.
const subjectJWTProperty = "jwt"

// doelbindingKey is the request-context entry through which a request picks
// the policy to evaluate, and doelbindingPrefix is the package namespace
// those policies live in.
const (
	doelbindingKey    = "doelbinding"
	doelbindingPrefix = "doelbinding"
)

// AccessServer serves the AuthZEN Access API from the policy engine.
//
// The directory serves the same API from its relationship graph. This implementation serves it from Rego, so that a policy decision can be asked
// for - and logged - under the AuthZEN information model, in a shape fixed enough to record as a conformant decision.
//
// The action names the rule, and the request's doelbinding names the package/ it lives in - see policyPath.
// With a bundle rooted at `package authz` and no doelbinding, an action.name of "request_laadpaal" evaluates data.authz.request_laadpaal.
// That rule returns either the AuthZEN decision object verbatim - {"decision": bool, "context": {...}} - or a bare boolean.
type AccessServer struct {
	authz *AuthorizerServer
}

var _ dsa.AccessServer = (*AccessServer)(nil)

func NewAccessServer(authz *AuthorizerServer) *AccessServer {
	return &AccessServer{authz: authz}
}

// Evaluation evaluates a single access request against the loaded policy.
func (s *AccessServer) Evaluation(ctx context.Context, req *dsa.EvaluationRequest) (*dsa.EvaluationResponse, error) {
	resp, meta, err := s.evaluation(ctx, req)

	return resp, s.authz.adlError(err, s.authz.adl.Evaluation(ctx, meta.request(req), resp, err))
}

// Evaluations evaluates a batch of access requests, each falling back to the
// batch-level subject, action, resource and context for whatever it leaves
// unset. Only the default "execute all" semantics are supported; the
// request's options are ignored.
func (s *AccessServer) Evaluations(
	ctx context.Context, req *dsa.EvaluationsRequest,
) (*dsa.EvaluationsResponse, error) {
	resp, meta, err := s.evaluations(ctx, req)

	return resp, s.authz.adlError(err, s.authz.adl.Evaluations(ctx, meta.requests(req), resp, err))
}

// SubjectSearch is not served by the policy engine. See errSearchUnsupported.
func (s *AccessServer) SubjectSearch(
	ctx context.Context, req *dsa.SubjectSearchRequest,
) (*dsa.SubjectSearchResponse, error) {
	resp, err := &dsa.SubjectSearchResponse{}, errSearchUnsupported("subject")

	return resp, s.authz.adlError(err, s.authz.adl.SubjectSearch(ctx, req, resp, err))
}

// ResourceSearch is not served by the policy engine. See errSearchUnsupported.
func (s *AccessServer) ResourceSearch(
	ctx context.Context, req *dsa.ResourceSearchRequest,
) (*dsa.ResourceSearchResponse, error) {
	resp, err := &dsa.ResourceSearchResponse{}, errSearchUnsupported("resource")

	return resp, s.authz.adlError(err, s.authz.adl.ResourceSearch(ctx, req, resp, err))
}

// ActionSearch is not served by the policy engine. See errSearchUnsupported.
func (s *AccessServer) ActionSearch(
	ctx context.Context, req *dsa.ActionSearchRequest,
) (*dsa.ActionSearchResponse, error) {
	resp, err := &dsa.ActionSearchResponse{}, errSearchUnsupported("action")

	return resp, s.authz.adlError(err, s.authz.adl.ActionSearch(ctx, req, resp, err))
}

func (s *AccessServer) evaluation(
	ctx context.Context, req *dsa.EvaluationRequest,
) (*dsa.EvaluationResponse, evalMeta, error) {
	rt, err := s.authz.getRuntime(ctx)
	if err != nil {
		return &dsa.EvaluationResponse{}, evalMeta{identity: identityContext(req.GetSubject())}, err
	}

	return s.evaluate(ctx, rt, req)
}

func (s *AccessServer) evaluations(
	ctx context.Context, req *dsa.EvaluationsRequest,
) (*dsa.EvaluationsResponse, evalMeta, error) {
	meta := evalMeta{identity: identityContext(req.GetSubject())}

	if len(req.GetEvaluations()) == 0 {
		return &dsa.EvaluationsResponse{}, meta, aerr.ErrInvalidArgument.Msg("evaluations not set")
	}

	rt, err := s.authz.getRuntime(ctx)
	if err != nil {
		return &dsa.EvaluationsResponse{}, meta, err
	}

	// A sub-request bringing no context of its own is evaluated against the
	// batch's policy, so that is the one the batch record names. When the
	// batch selects none - because its sub-requests each select their own -
	// the record names no policy rather than one of theirs.
	meta.policyPath, _ = s.policyPath(ctx, rt, req.GetContext())

	resp := &dsa.EvaluationsResponse{
		Evaluations: make([]*dsa.EvaluationResponse, 0, len(req.GetEvaluations())),
	}

	for _, evaluation := range req.GetEvaluations() {
		outcome, outcomeMeta, err := s.evaluate(ctx, rt, defaulted(req, evaluation))
		if err != nil {
			return resp, meta, err
		}

		// The batch subject is only what this sub-request resolved to when
		// the sub-request did not bring a subject of its own.
		if meta.user == nil && evaluation.GetSubject() == nil {
			meta.user = outcomeMeta.user
		}

		resp.Evaluations = append(resp.GetEvaluations(), outcome)
	}

	return resp, meta, nil
}

func (s *AccessServer) evaluate(
	ctx context.Context, rt *runtime.Runtime, req *dsa.EvaluationRequest,
) (*dsa.EvaluationResponse, evalMeta, error) {
	meta := evalMeta{identity: identityContext(req.GetSubject())}

	action := req.GetAction().GetName()
	if action == "" {
		return &dsa.EvaluationResponse{}, meta, aerr.ErrInvalidArgument.Msg("action name not set")
	}

	path, err := s.policyPath(ctx, rt, req.GetContext())
	if err != nil {
		return &dsa.EvaluationResponse{}, meta, err
	}

	meta.policyPath = path

	input, user, err := s.input(ctx, req, meta.identity)
	if err != nil {
		return &dsa.EvaluationResponse{}, meta, err
	}

	meta.user = user

	s.authz.logger.Debug().Str("api", "evaluation").Str("action", action).
		Interface("input", input).Msg("evaluation")

	query, err := s.authz.preparedQueries.decisionQuery(ctx, rt, path, []string{action})
	if err != nil {
		return &dsa.EvaluationResponse{}, meta, err
	}

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return &dsa.EvaluationResponse{}, meta,
			aerr.ErrBadQuery.Err(err).Msgf("query evaluation failed: path=%s action=%s", path, action)
	}

	if len(results) == 0 {
		return &dsa.EvaluationResponse{}, meta,
			aerr.ErrBadQuery.Msgf("undefined results: path=%s action=%s", path, action)
	}

	binding, ok := results[0].Bindings[bindingName(0)]
	if !ok {
		return &dsa.EvaluationResponse{}, meta, aerr.ErrBadQuery.Msgf("failed getting binding for action [%s]", action)
	}

	resp, err := evaluationResponse(action, binding)

	return resp, meta, err
}

// input builds the Rego input for an AuthZEN evaluation.
//
// input.user and input.identity keep a stable shape so that a policy written against them keeps working.
// The AuthZEN model is added alongside, with each element's properties flattened and its structural fields folded in: a policy reads
// input.resource.postcode, not input.resource.properties.postcode.
func (s *AccessServer) input(
	ctx context.Context, req *dsa.EvaluationRequest, identity *api.IdentityContext,
) (map[string]any, *dsc.Object, error) {
	input := map[string]any{}

	user, err := s.authz.resolveIdentityContext(ctx, identity, input)
	if err != nil {
		return nil, nil, err
	}

	input[InputSubject] = flatten(req.GetSubject().GetProperties(), map[string]string{
		typeField: req.GetSubject().GetType(),
		idField:   req.GetSubject().GetId(),
	})

	input[InputAction] = flatten(req.GetAction().GetProperties(), map[string]string{
		nameField: req.GetAction().GetName(),
	})

	input[InputResource] = flatten(req.GetResource().GetProperties(), map[string]string{
		typeField: req.GetResource().GetType(),
		idField:   req.GetResource().GetId(),
	})

	input[InputContext] = req.GetContext().AsMap()

	return input, user, nil
}

// policyPath returns the package to evaluate a request against.
//
// AuthZEN has no policy field, so a request selects its policy through the context: `"doelbinding": "laadpalen"` evaluates
// data.doelbinding.laadpalen.
// Selectable policies live under that one prefix, so that a request cannot reach a library package by naming it, and an unknown doelbinding is an
// error rather than a policy chosen on the caller's behalf.
//
// A request that selects nothing gets the policy the instance was configured to serve, which is the whole story for a single-policy deployment.
func (s *AccessServer) policyPath(
	ctx context.Context, rt *runtime.Runtime, reqContext *structpb.Struct,
) (string, error) {
	packages, err := s.authz.preparedQueries.policyPackages(ctx, rt)
	if err != nil {
		return "", aerr.ErrBadRuntime.Err(err).Msg("failed to list the loaded policies")
	}

	selected := stringProperty(reqContext, doelbindingKey)
	if selected == "" {
		policyRoot, err := rt.SelectPolicyRoot(defaultPolicyRoots(packages))
		if err != nil {
			return "", aerr.ErrBadRuntime.Err(err).Msg("no policy to evaluate")
		}

		return policyRoot, nil
	}

	return doelbindingPolicy(packages, selected)
}

// defaultPolicyRoots returns the roots a request that selects nothing can be served from.
// The doelbinding namespace is not among them: those packages are reachable only by naming one, so falling back into the namespace would answer with
// a policy the request did not ask for - and with the bare `doelbinding` root, which is no policy at all.
func defaultPolicyRoots(packages []string) []string {
	return slices.DeleteFunc(runtime.PolicyRoots(packages), func(root string) bool {
		return root == doelbindingPrefix
	})
}

// doelbindingPolicy maps a selected doelbinding onto the loaded package that serves it.
// Prefixing is what contains the selection: no doelbinding can name a package outside the namespace set aside for them.
func doelbindingPolicy(packages []string, selected string) (string, error) {
	path := doelbindingPrefix + "." + selected

	if !slices.Contains(packages, path) {
		return "", aerr.ErrInvalidArgument.Msgf(
			"doelbinding %q selects no loaded policy; expected a bundle carrying `package %s`", selected, path)
	}

	return path, nil
}

// evalMeta carries what the decision log needs but an AuthZEN request does not hold: which policy the decision came from, and what the subject
// actually resolved to.
type evalMeta struct {
	policyPath string
	identity   *api.IdentityContext
	user       *dsc.Object
}

// request returns the form of req that goes into the decision log: the subject replaced by what the identity resolved to - never the caller's
// properties, which may carry a bearer token - and the deciding policy recorded in the context.
func (m evalMeta) request(req *dsa.EvaluationRequest) *dsa.EvaluationRequest {
	return &dsa.EvaluationRequest{
		Subject:  adlSubject(m.identity, m.user),
		Action:   req.GetAction(),
		Resource: req.GetResource(),
		Context:  withPolicyPath(req.GetContext(), m.policyPath),
	}
}

func (m evalMeta) requests(req *dsa.EvaluationsRequest) *dsa.EvaluationsRequest {
	evaluations := make([]*dsa.EvaluationRequest, 0, len(req.GetEvaluations()))

	for _, evaluation := range req.GetEvaluations() {
		evaluations = append(evaluations, &dsa.EvaluationRequest{
			Subject:  scrubSubject(evaluation.GetSubject()),
			Action:   evaluation.GetAction(),
			Resource: evaluation.GetResource(),
			Context:  evaluation.GetContext(),
		})
	}

	return &dsa.EvaluationsRequest{
		Subject:     adlSubject(m.identity, m.user),
		Action:      req.GetAction(),
		Resource:    req.GetResource(),
		Context:     withPolicyPath(req.GetContext(), m.policyPath),
		Evaluations: evaluations,
	}
}

// identityContext maps an AuthZEN subject onto the identity context the authorizer resolves directory users from.
func identityContext(subject *dsa.Subject) *api.IdentityContext {
	if jwt := stringProperty(subject.GetProperties(), subjectJWTProperty); jwt != "" {
		return &api.IdentityContext{Type: api.IdentityType_IDENTITY_TYPE_JWT, Identity: jwt}
	}

	if id := subject.GetId(); id != "" {
		return &api.IdentityContext{Type: api.IdentityType_IDENTITY_TYPE_SUB, Identity: id}
	}

	return &api.IdentityContext{Type: api.IdentityType_IDENTITY_TYPE_NONE}
}

// evaluationResponse maps the value a decision rule bound onto an AuthZEN evaluation response. A rule either returns the decision object verbatim,
// or a bare boolean when it carries nothing but an outcome.
func evaluationResponse(action string, binding any) (*dsa.EvaluationResponse, error) {
	switch result := binding.(type) {
	case bool:
		return &dsa.EvaluationResponse{Decision: result}, nil

	case map[string]any:
		decision, ok := result[decisionField].(bool)
		if !ok {
			return &dsa.EvaluationResponse{}, aerr.ErrBadQuery.Msgf(
				"rule for action [%s] returned an object without a boolean %q field", action, decisionField)
		}

		resp := &dsa.EvaluationResponse{Decision: decision}

		decisionContext, ok := result[contextField].(map[string]any)
		if !ok {
			return resp, nil
		}

		properties, err := structpb.NewStruct(decisionContext)
		if err != nil {
			return &dsa.EvaluationResponse{}, aerr.ErrBadQuery.Err(err).Msgf(
				"rule for action [%s] returned a %q that is not representable as JSON", action, contextField)
		}

		resp.Context = properties

		return resp, nil

	default:
		return &dsa.EvaluationResponse{}, aerr.ErrBadQuery.Msgf(
			"rule for action [%s] returned %T, want a boolean or a decision object", action, binding)
	}
}

// defaulted fills a sub-request's unset fields from the batch-level defaults.
func defaulted(batch *dsa.EvaluationsRequest, req *dsa.EvaluationRequest) *dsa.EvaluationRequest {
	return &dsa.EvaluationRequest{
		Subject:  cmp.Or(req.GetSubject(), batch.GetSubject()),
		Action:   cmp.Or(req.GetAction(), batch.GetAction()),
		Resource: cmp.Or(req.GetResource(), batch.GetResource()),
		Context:  cmp.Or(req.GetContext(), batch.GetContext()),
	}
}

// flatten merges an AuthZEN properties bag with the structural fields of the element it belongs to. A property already using one of those names
// wins: caller data is never silently overwritten.
func flatten(properties *structpb.Struct, fields map[string]string) map[string]any {
	out := properties.AsMap()

	for name, value := range fields {
		if value == "" {
			continue
		}

		if _, taken := out[name]; !taken {
			out[name] = value
		}
	}

	return out
}

// withPolicyPath records which policy produced the decision. AuthZEN has no field for it - the request names a doelbinding, not a package - and
// without it a log record cannot be tied back to the rule that decided.
func withPolicyPath(base *structpb.Struct, policyPath string) *structpb.Struct {
	if policyPath == "" {
		return base
	}

	fields := make(map[string]*structpb.Value, len(base.GetFields())+1)
	maps.Copy(fields, base.GetFields())
	fields[policyPathKey] = structpb.NewStringValue(policyPath)

	return &structpb.Struct{Fields: fields}
}

// scrubSubject drops the caller's subject properties, which may carry a bearer token, keeping only the type and id the decision was made for.
func scrubSubject(subject *dsa.Subject) *dsa.Subject {
	if subject == nil {
		return nil
	}

	return &dsa.Subject{Type: subject.GetType(), Id: subject.GetId()}
}

func stringProperty(properties *structpb.Struct, key string) string {
	return properties.GetFields()[key].GetStringValue()
}

// errSearchUnsupported reports that a search API has no policy-engine implementation.
// The searches enumerate candidates, which the directory can do by walking its relationship graph but a Rego rule cannot: a policy answers
// "may this subject do this?", and offers no way to enumerate the subjects, resources or actions it would admit. They are served by the
// directory's Access API instead.
func errSearchUnsupported(api string) error {
	return status.Errorf(codes.Unimplemented,
		"%s search is not available from the policy engine; use the directory's Access API", api)
}
