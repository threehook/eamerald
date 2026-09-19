package adl

import (
	"time"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/threehook/eamerald/internal/header"
	"google.golang.org/protobuf/types/known/structpb"
)

// Status is the ADL decision status. A denied access evaluation is Ok;
// Error is reserved for the PDP failing to reach a decision at all.
type Status string

const (
	StatusOk    Status = "Ok"
	StatusError Status = "Error"
)

// The five conformant event_name values, one per AuthZEN API. Every record
// this package emits carries exactly one of them, selected by the endpoint
// that produced the decision rather than inferred from its shape.
const (
	EventAccessEvaluation  = "adl.access_evaluation"
	EventAccessEvaluations = "adl.access_evaluations"
	EventSearchSubject     = "adl.search_subject"
	EventSearchAction      = "adl.search_action"
	EventSearchResource    = "adl.search_resource"
)

// Record is a Logius ADL 1.0 Level 1 record: trace/span correlation, the
// event name, timestamp, status, producer identity, and the AuthZEN
// request/response body. No policy/information/configuration source
// references - those start at Level 2.
type Record struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	EventName    string            `json:"event_name"`
	Timestamp    int64             `json:"timestamp"`
	Status       Status            `json:"status"`
	Attributes   map[string]any    `json:"attributes"`
	Resource     map[string]string `json:"resource,omitempty"`
	Body         Body              `json:"body"`
}

// Body carries the AuthZEN request/response under their fixed spec keys.
//
//nolint:tagliatelle // "adl.core.request"/"adl.core.response" are literal keys mandated by the ADL 1.0 spec, not a naming style choice.
type Body struct {
	Request  any `json:"adl.core.request,omitempty"`
	Response any `json:"adl.core.response,omitempty"`
}

// evaluationResponse mirrors dsa.EvaluationResponse's JSON shape without its
// "omitempty" on Decision: Go's encoding/json treats false as empty, which
// silently drops "decision" from a denied evaluation - indistinguishable
// from a record that never reached a decision at all.
type evaluationResponse struct {
	Decision bool             `json:"decision"`
	Context  *structpb.Struct `json:"context,omitempty"`
}

// evaluationsResponse is evaluationResponse's batch-shape counterpart.
type evaluationsResponse struct {
	Evaluations []evaluationResponse `json:"evaluations"`
}

// responseForJSON rewrites an AuthZEN evaluation response into the shape it
// must be recorded in, so a denied decision's "decision": false is never
// dropped by the vendored proto types' own json tags. Every other response
// type (the search APIs carry no bare Decision bool) is recorded as-is.
func responseForJSON(response any) any {
	switch r := response.(type) {
	case *dsa.EvaluationResponse:
		return evaluationResponse{Decision: r.GetDecision(), Context: r.GetContext()}

	case *dsa.EvaluationsResponse:
		evaluations := make([]evaluationResponse, 0, len(r.GetEvaluations()))
		for _, e := range r.GetEvaluations() {
			evaluations = append(evaluations, evaluationResponse{Decision: e.GetDecision(), Context: e.GetContext()})
		}

		return evaluationsResponse{Evaluations: evaluations}

	default:
		return response
	}
}

// record builds the envelope shared by every event type. When evalErr is
// non-nil the PDP never reached a decision: the record gets status Error and
// carries only the request, showing what was attempted.
func (l *Logger) record(tc header.TraceContext, eventName string, request, response any, evalErr error) Record {
	rec := Record{
		TraceID:      tc.TraceID,
		SpanID:       tc.SpanID,
		ParentSpanID: tc.ParentSpanID,
		EventName:    eventName,
		Timestamp:    time.Now().UTC().UnixMilli(),
		Status:       StatusOk,
		Attributes:   map[string]any{},
		Resource:     l.cfg.Resource,
		Body:         Body{Request: request},
	}

	if evalErr != nil {
		rec.Status = StatusError
		return rec
	}

	rec.Body.Response = responseForJSON(response)

	return rec
}

// AuthZENResource maps a Topaz resource context onto an AuthZEN resource.
// AuthZEN requires a resource type, which a free-form Topaz resource context
// has no fixed place for, so the type and id are lifted out of the
// configured keys; the whole context is kept as the resource properties
// either way.
func (c Config) AuthZENResource(props *structpb.Struct) *dsa.Resource {
	return &dsa.Resource{
		Type:       stringField(props, c.ResourceContext.typeKey()),
		Id:         stringField(props, c.ResourceContext.idKey()),
		Properties: props,
	}
}

func stringField(props *structpb.Struct, key string) string {
	if props == nil {
		return ""
	}

	v, ok := props.GetFields()[key]
	if !ok {
		return ""
	}

	return v.GetStringValue()
}
