package impl

import (
	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
)

// policyPathKey names the request-context entry carrying the evaluated
// policy path.
const policyPathKey = "policy_path"

// adlError decides what the Access API returns when writing the decision
// record failed. A genuine evaluation error always wins - it must not be
// masked by a logging failure - but an otherwise successful evaluation
// fails, so that no decision reaches a PEP without a log record.
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
