package impl

import (
	"context"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// policyPathKey names the request-context entry carrying the evaluated
// policy path.
const policyPathKey = "policy_path"

// logADLDecision writes the Authorization Decision Log record for one Is()
// call.
//
// Is() is not an AuthZEN endpoint, so the decision is recorded under the
// AuthZEN model whose information model it corresponds to, as the spec
// directs for OPA-style direct calls: a single requested decision is an
// Access Evaluation, a list of them is an Access Evaluations call against
// one subject and resource.
func (s *AuthorizerServer) logADLDecision(
	ctx context.Context,
	req *authorizer.IsRequest,
	user *dsc.Object,
	resp *authorizer.IsResponse,
	evalErr error,
) error {
	decisions := req.GetPolicyContext().GetDecisions()
	subject := adlSubject(req.GetIdentityContext(), user)
	resource := s.adl.Config().AuthZENResource(req.GetResourceContext())
	reqContext := adlRequestContext(req.GetPolicyContext())

	if len(decisions) == 1 {
		return s.adl.Evaluation(ctx,
			&dsa.EvaluationRequest{
				Subject:  subject,
				Action:   &dsa.Action{Name: decisions[0]},
				Resource: resource,
				Context:  reqContext,
			},
			&dsa.EvaluationResponse{Decision: firstOutcome(resp)},
			evalErr,
		)
	}

	evaluations := make([]*dsa.EvaluationRequest, 0, len(decisions))
	for _, d := range decisions {
		evaluations = append(evaluations, &dsa.EvaluationRequest{Action: &dsa.Action{Name: d}})
	}

	outcomes := make([]*dsa.EvaluationResponse, 0, len(resp.GetDecisions()))
	for _, d := range resp.GetDecisions() {
		outcomes = append(outcomes, &dsa.EvaluationResponse{Decision: d.GetIs()})
	}

	return s.adl.Evaluations(ctx,
		&dsa.EvaluationsRequest{
			Subject:     subject,
			Resource:    resource,
			Context:     reqContext,
			Evaluations: evaluations,
		},
		&dsa.EvaluationsResponse{Evaluations: outcomes},
		evalErr,
	)
}

func firstOutcome(resp *authorizer.IsResponse) bool {
	if outcomes := resp.GetDecisions(); len(outcomes) > 0 {
		return outcomes[0].GetIs()
	}

	return false
}

// adlError decides what Is() returns when writing the decision record
// failed. A genuine evaluation error always wins - it must not be masked by
// a logging failure - but an otherwise successful evaluation fails, so that
// no decision reaches a PEP without a log record.
func (s *AuthorizerServer) adlError(evalErr, logErr error) error {
	if logErr == nil {
		return evalErr
	}

	if evalErr != nil {
		s.logger.Error().Err(logErr).Msg("failed to write adl decision record")
		return evalErr
	}

	return logErr
}

// adlSubject maps a Topaz identity context onto an AuthZEN subject.
//
// When the identity resolved to a directory user, that object is the
// subject: its type and id are what the policy was evaluated against.
// Before resolution there is only the identity context, and for
// IDENTITY_TYPE_JWT its identity value is the bearer token itself - the id
// is then left empty rather than writing a live credential to the log.
func adlSubject(identityContext *api.IdentityContext, user *dsc.Object) *dsa.Subject {
	if user != nil {
		return &dsa.Subject{Type: user.GetType(), Id: user.GetId()}
	}

	subject := &dsa.Subject{Type: identityContext.GetType().String()}

	if identityContext.GetType() != api.IdentityType_IDENTITY_TYPE_JWT {
		subject.Id = identityContext.GetIdentity()
	}

	return subject
}

// adlRequestContext records which policy produced the decision. AuthZEN has
// no field for it - the API assumes one PDP evaluates one policy - but
// Topaz takes a policy path per request, so without it a log record cannot
// be tied back to the rule that decided.
func adlRequestContext(policyContext *api.PolicyContext) *structpb.Struct {
	if policyContext.GetPath() == "" {
		return nil
	}

	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			policyPathKey: structpb.NewStringValue(policyContext.GetPath()),
		},
	}
}
