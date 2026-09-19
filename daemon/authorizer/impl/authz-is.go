package impl

import (
	"context"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	"github.com/aserto-dev/go-directory/pkg/pb"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Is evaluates the decisions named in the request's policy context.
//
// Exactly one Authorization Decision Log record is written per call -
// including for requests the PDP could not evaluate at all - so every exit
// from the evaluation passes through here. The evaluation itself is in is().
func (s *AuthorizerServer) Is(ctx context.Context, req *authorizer.IsRequest) (*authorizer.IsResponse, error) {
	resp, user, err := s.is(ctx, req)

	return resp, s.adlError(err, s.logADLDecision(ctx, req, user, resp, err))
}

// is evaluates the request and returns the directory user the identity
// resolved to, which the decision log records as the AuthZEN subject.
func (s *AuthorizerServer) is(
	ctx context.Context, req *authorizer.IsRequest,
) (*authorizer.IsResponse, *dsc.Object, error) {
	log := s.logger.With().Str("api", "is").Logger()

	if err := s.isVerifyRequest(req); err != nil {
		return &authorizer.IsResponse{}, nil, err
	}

	input, user, err := s.isSetInput(ctx, req)
	if err != nil {
		return &authorizer.IsResponse{}, nil, err
	}

	log.Debug().Interface("input", input).Msg("is")

	rt, err := s.getRuntime(ctx)
	if err != nil {
		return &authorizer.IsResponse{}, user, err
	}

	policyPath := req.GetPolicyContext().GetPath()
	decisions := req.GetPolicyContext().GetDecisions()

	query, err := s.preparedQueries.decisionQuery(ctx, rt, policyPath, decisions)
	if err != nil {
		return &authorizer.IsResponse{}, user, err
	}

	resp := &authorizer.IsResponse{
		Decisions: []*authorizer.Decision{},
	}

	queryResults, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return resp, user, aerr.ErrBadQuery.Err(err).Msgf("query evaluation failed: path=%s decisions=%v", policyPath, decisions)
	}

	if len(queryResults) == 0 {
		return resp, user, aerr.ErrBadQuery.Msgf("undefined results: path=%s decisions=%v", policyPath, decisions)
	}

	for i, d := range decisions {
		v, ok := queryResults[0].Bindings[bindingName(i)]
		if !ok {
			return resp, user, aerr.ErrBadQuery.Msgf("failed getting binding for decision [%s]", d)
		}

		outcome, ok := v.(bool)
		if !ok {
			return resp, user, aerr.ErrBadQuery.Msgf("non-boolean outcome for decision [%s]: %v", d, v)
		}

		decision := authorizer.Decision{
			Decision: d,
			Is:       outcome,
		}

		resp.Decisions = append(resp.GetDecisions(), &decision)
	}

	return resp, user, nil
}

func (*AuthorizerServer) isVerifyRequest(req *authorizer.IsRequest) error {
	if req.GetPolicyContext() == nil {
		return aerr.ErrInvalidArgument.Msg("policy context not set")
	}

	if req.GetPolicyContext().GetPath() == "" {
		return aerr.ErrInvalidArgument.Msg("policy context path not set")
	}

	if len(req.GetPolicyContext().GetDecisions()) == 0 {
		return aerr.ErrInvalidArgument.Msg("policy context decisions not set")
	}

	if req.GetResourceContext() == nil {
		req.ResourceContext = pb.NewStruct()
	}

	if req.GetIdentityContext() == nil {
		return aerr.ErrInvalidArgument.Msg("identity context not set")
	}

	if req.GetIdentityContext().GetType() == api.IdentityType_IDENTITY_TYPE_UNKNOWN {
		return aerr.ErrInvalidArgument.Msg("identity type UNKNOWN")
	}

	return nil
}

func (s *AuthorizerServer) isSetInput(
	ctx context.Context, req *authorizer.IsRequest,
) (map[string]any, *dsc.Object, error) {
	input := map[string]any{}

	user, err := s.resolveIdentityContext(ctx, req.GetIdentityContext(), input)
	if err != nil {
		return nil, nil, err
	}

	if req.GetPolicyContext() != nil {
		input[InputPolicy] = req.GetPolicyContext()
	}

	if req.GetResourceContext() != nil {
		input[InputResource] = req.GetResourceContext()
	}

	return input, user, nil
}
