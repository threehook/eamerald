package adl_decision_logger

import (
	"time"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/aserto-dev/topaz/internal/header"
	dsa "github.com/authzen/access.go/api/access/v1"
)

// Status is the Logius ADL 1.0 §3.3.5 decision status. A denied access
// evaluation is Ok; Error is reserved for the PDP failing to reach a
// decision at all.
type Status string

const (
	StatusOk    Status = "Ok"
	StatusError Status = "Error"
)

// event_name values for the two AuthZEN access-evaluation shapes,
// see https://gitdocumentatie.logius.nl/publicatie/ftv/adl/1.0.0/ §3.3.3.
const (
	EventNameAccessEvaluation  = "adl.access_evaluation"
	EventNameAccessEvaluations = "adl.access_evaluations"
)

// Record is a Logius ADL 1.0 Level 1 record: trace/span correlation, a fixed
// event name, timestamp, status, and the AuthZEN request/response body. No
// policy/information/configuration references (those start at Level 2+).
type Record struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	EventName    string         `json:"event_name"`
	Timestamp    int64          `json:"timestamp"`
	Status       Status         `json:"status"`
	Attributes   map[string]any `json:"attributes"`
	Body         Body           `json:"body"`
}

// Body carries the AuthZEN request/response under their §3.3.8 fixed keys.
//
//nolint:tagliatelle // "adl.core.request"/"adl.core.response" are literal keys mandated by the ADL 1.0 spec, not a naming style choice.
type Body struct {
	Request  any `json:"adl.core.request,omitempty"`
	Response any `json:"adl.core.response,omitempty"`
}

// buildRecord maps a Topaz Is() call onto an ADL Level 1 record. A single
// requested decision maps to the singular AuthZEN evaluation shape; more than
// one requested decision (Topaz's Is() accepts a list of decision names
// evaluated against one identity/resource context) maps to the batch
// evaluations shape.
func buildRecord(tc header.TraceContext, req *authorizer.IsRequest, decisions []*authorizer.Decision) Record {
	record := Record{
		TraceID:      tc.TraceID,
		SpanID:       tc.SpanID,
		ParentSpanID: tc.ParentSpanID,
		Timestamp:    time.Now().UTC().UnixMilli(),
		Status:       StatusOk,
		Attributes:   map[string]any{},
	}

	subject := &dsa.Subject{
		Type: req.GetIdentityContext().GetType().String(),
		Id:   req.GetIdentityContext().GetIdentity(),
	}
	resource := &dsa.Resource{
		Properties: req.GetResourceContext(),
	}

	if len(decisions) == 1 {
		record.EventName = EventNameAccessEvaluation
		record.Body = Body{
			Request: &dsa.EvaluationRequest{
				Subject:  subject,
				Action:   &dsa.Action{Name: decisions[0].GetDecision()},
				Resource: resource,
			},
			Response: &dsa.EvaluationResponse{
				Decision: decisions[0].GetIs(),
			},
		}

		return record
	}

	evalReqs := make([]*dsa.EvaluationRequest, 0, len(decisions))
	evalResps := make([]*dsa.EvaluationResponse, 0, len(decisions))

	for _, d := range decisions {
		evalReqs = append(evalReqs, &dsa.EvaluationRequest{
			Action: &dsa.Action{Name: d.GetDecision()},
		})
		evalResps = append(evalResps, &dsa.EvaluationResponse{
			Decision: d.GetIs(),
		})
	}

	record.EventName = EventNameAccessEvaluations
	record.Body = Body{
		Request: &dsa.EvaluationsRequest{
			Subject:     subject,
			Resource:    resource,
			Evaluations: evalReqs,
		},
		Response: &dsa.EvaluationsResponse{
			Evaluations: evalResps,
		},
	}

	return record
}
