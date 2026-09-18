package impl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	"github.com/aserto-dev/go-authorizer/pkg/aerr"
	"github.com/aserto-dev/go-directory/pkg/pb"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/adl_decision_logger"
	"github.com/threehook/eamerald/internal/runtime"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/pkg/errors"
)

//nolint:funlen
func (s *AuthorizerServer) Is(ctx context.Context, req *authorizer.IsRequest) (*authorizer.IsResponse, error) {
	log := s.logger.With().Str("api", "is").Logger()

	if err := s.isVerifyRequest(req); err != nil {
		s.logADLEvaluationErrorDirect(ctx, req)
		return &authorizer.IsResponse{}, err
	}

	input, err := s.isSetInput(ctx, req)
	if err != nil {
		s.logADLEvaluationErrorDirect(ctx, req)
		return &authorizer.IsResponse{}, err
	}

	log.Debug().Interface("input", input).Msg("is")

	rt, err := s.getRuntime(ctx)
	if err != nil {
		s.logADLEvaluationErrorDirect(ctx, req)
		return &authorizer.IsResponse{}, err
	}

	// The Rego query body and its prepared form depend only on the policy
	// path and the decisions list — both stable for the lifetime of the
	// active OPA compiler. Cache the PreparedEvalQuery so repeated Is()
	// calls for the same (path, decisions) skip the parse + plan work and
	// stop fighting each other on the compiler's internal locks. The cache
	// is invalidated whenever the compiler is rotated (bundle reload).
	policyPath := req.GetPolicyContext().GetPath()
	decisions := req.GetPolicyContext().GetDecisions()
	preparedKey := cacheKey(policyPath, decisions)

	query, err := s.preparedQueries.getOrPrepare(ctx, rt, preparedKey, func(ctx context.Context) (rego.PreparedEvalQuery, error) {
		queryStmt := strings.Builder{}

		for i, decision := range decisions {
			rule := fmt.Sprintf("data.%s.%s\n", policyPath, decision)

			if ok, err := rt.ValidateRule(rule); !ok {
				return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid rule: %q", rule)
			}

			q := fmt.Sprintf("x%d = %s\n", i, rule)

			if _, err := rt.ValidateQuery(q); err != nil {
				return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid query: %q", q)
			}

			queryStmt.WriteString(q)
		}

		pq, err := rt.ValidateQuery(queryStmt.String())
		if err != nil {
			return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msgf("invalid query batch: %q", queryStmt.String())
		}

		prepared, err := rego.New(
			rego.Compiler(rt.GetPluginsManager().GetCompiler()),
			rego.Store(rt.GetPluginsManager().Store),
			rego.ParsedQuery(pq),
		).PrepareForEval(ctx)
		if err != nil {
			return rego.PreparedEvalQuery{}, aerr.ErrBadQuery.Err(err).Msg(queryStmt.String())
		}

		return prepared, nil
	})
	if err != nil {
		s.logADLEvaluationError(ctx, rt, req)
		return &authorizer.IsResponse{}, err
	}

	resp := &authorizer.IsResponse{
		Decisions: []*authorizer.Decision{},
	}

	queryResults, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		s.logADLEvaluationError(ctx, rt, req)
		return resp, aerr.ErrBadQuery.Err(err).Msgf("query evaluation failed: path=%s decisions=%v", policyPath, decisions)
	}

	if len(queryResults) == 0 {
		s.logADLEvaluationError(ctx, rt, req)
		return resp, aerr.ErrBadQuery.Err(err).Msgf("undefined results: path=%s decisions=%v", policyPath, decisions)
	}

	for i, d := range req.GetPolicyContext().GetDecisions() {
		v, ok := queryResults[0].Bindings[fmt.Sprintf("x%d", i)]
		if !ok {
			return nil, errors.Wrapf(err, "failed getting binding for decision [%s]", d)
		}

		outcome, ok := v.(bool)
		if !ok {
			return nil, errors.Wrapf(err, "non-boolean outcome for decision [%s]: %s", d, v)
		}

		decision := authorizer.Decision{
			Decision: d,
			Is:       outcome,
		}

		resp.Decisions = append(resp.GetDecisions(), &decision)
	}

	if adlPlugin := adl_decision_logger.Lookup(rt.GetPluginsManager()); adlPlugin != nil {
		if err := adlPlugin.LogDecision(ctx, req, resp.GetDecisions()); err != nil {
			return resp, err
		}
	}

	return resp, err
}

// logADLEvaluationErrorDirect records an ADL status-Error entry for an Is()
// failure that occurs before an OPA runtime is resolved, so there is no
// plugins.Manager to look the adl_decision_logger plugin instance up from.
// Per the ADL 1.0 spec (§3.3.9), every evaluated request - including ones
// the PDP could not complete - MUST produce exactly one log record; a
// logging failure here is itself logged, but the original error from the
// caller is always what gets returned, not this one.
func (s *AuthorizerServer) logADLEvaluationErrorDirect(ctx context.Context, req *authorizer.IsRequest) {
	rawConfig, ok := s.cfg.OPA.Config.Plugins[adl_decision_logger.PluginName]
	if !ok || !adl_decision_logger.IsEnabled(rawConfig) {
		return
	}

	if err := adl_decision_logger.LogEvaluationErrorDirect(ctx, req); err != nil {
		s.logger.Error().Err(err).Msg("failed to write adl evaluation-error record")
	}
}

// logADLEvaluationError records an ADL status-Error entry for an Is()
// failure that occurs after an OPA runtime was resolved (query preparation,
// evaluation, or undefined results) - see logADLEvaluationErrorDirect for
// failures before that point.
func (s *AuthorizerServer) logADLEvaluationError(ctx context.Context, rt *runtime.Runtime, req *authorizer.IsRequest) {
	adlPlugin := adl_decision_logger.Lookup(rt.GetPluginsManager())
	if adlPlugin == nil {
		return
	}

	if err := adlPlugin.LogEvaluationError(ctx, req); err != nil {
		s.logger.Error().Err(err).Msg("failed to write adl evaluation-error record")
	}
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

func (s *AuthorizerServer) isSetInput(ctx context.Context, req *authorizer.IsRequest) (map[string]any, error) {
	input := map[string]any{}

	if err := s.resolveIdentityContext(ctx, req.GetIdentityContext(), input); err != nil {
		return nil, err
	}

	if req.GetPolicyContext() != nil {
		input[InputPolicy] = req.GetPolicyContext()
	}

	if req.GetResourceContext() != nil {
		input[InputResource] = req.GetResourceContext()
	}

	return input, nil
}
