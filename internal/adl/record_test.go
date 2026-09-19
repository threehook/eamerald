//nolint:testpackage // record()/AuthZENResource internals are unexported and only need to be verified from within the package.
package adl

import (
	"encoding/json"
	"testing"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threehook/eamerald/internal/header"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	testOTLPEndpoint = "localhost:4317"
	testServiceKey   = "service.name"
	testServiceName  = "eamerald"
	testTypeKey      = "kind"
	testIDKey        = "key"
)

func testTraceContext() header.TraceContext {
	return header.TraceContext{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "",
	}
}

func testEvaluationRequest() *dsa.EvaluationRequest {
	return &dsa.EvaluationRequest{
		Subject:  &dsa.Subject{Type: "user", Id: "alice"},
		Action:   &dsa.Action{Name: "approve"},
		Resource: &dsa.Resource{Type: "holiday-request", Id: "446epbc8y7"},
	}
}

func marshaled(t *testing.T, record Record) map[string]any {
	t.Helper()

	buf, err := json.Marshal(record)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf, &m))

	return m
}

func TestRecord_MandatoryFieldsPresent(t *testing.T) {
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluation,
		testEvaluationRequest(), &dsa.EvaluationResponse{Decision: true}, nil)

	m := marshaled(t, rec)

	for _, field := range []string{"trace_id", "span_id", "event_name", "timestamp", "status", "attributes", "body"} {
		assert.Contains(t, m, field)
	}

	assert.Equal(t, string(StatusOk), m["status"])
	assert.NotContains(t, m, "parent_span_id", "empty parent_span_id must be omitted, not emitted as an empty string")

	body, ok := m["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "adl.core.request")
	assert.Contains(t, body, "adl.core.response")
}

func TestRecord_ParentSpanIDPresentWhenNotTraceRoot(t *testing.T) {
	logger := &Logger{}

	tc := testTraceContext()
	tc.ParentSpanID = "c4e1d75a3f9b8240"

	rec := logger.record(tc, EventAccessEvaluation, testEvaluationRequest(), &dsa.EvaluationResponse{}, nil)

	assert.Equal(t, "c4e1d75a3f9b8240", marshaled(t, rec)["parent_span_id"])
}

func TestRecord_EvaluationErrorYieldsStatusErrorWithoutResponse(t *testing.T) {
	// The response may be omitted when status is Error: the PDP never
	// reached a decision, so there is nothing to report. The request is
	// still recorded, to show what was attempted.
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluation,
		testEvaluationRequest(), &dsa.EvaluationResponse{Decision: true}, errors.New("engine fault"))

	assert.Equal(t, StatusError, rec.Status)

	body, ok := marshaled(t, rec)["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "adl.core.request")
	assert.NotContains(t, body, "adl.core.response")
}

func TestRecord_DenialIsNotAnError(t *testing.T) {
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluation,
		testEvaluationRequest(), &dsa.EvaluationResponse{Decision: false}, nil)

	assert.Equal(t, StatusOk, rec.Status)
}

func TestRecord_DeniedDecision_JSONIncludesDecisionFalse(t *testing.T) {
	// Regression: dsa.EvaluationResponse's generated json tag marks Decision
	// "omitempty", which drops a denied ("false") decision from the record
	// entirely - indistinguishable from one that never reached a decision.
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluation,
		testEvaluationRequest(), &dsa.EvaluationResponse{Decision: false}, nil)

	body, ok := marshaled(t, rec)["body"].(map[string]any)
	require.True(t, ok)
	resp, ok := body["adl.core.response"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, resp["decision"])
}

func TestRecord_DeniedBatchEvaluation_JSONIncludesDecisionFalse(t *testing.T) {
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluations,
		&dsa.EvaluationsRequest{}, &dsa.EvaluationsResponse{
			Evaluations: []*dsa.EvaluationResponse{{Decision: true}, {Decision: false}},
		}, nil)

	body, ok := marshaled(t, rec)["body"].(map[string]any)
	require.True(t, ok)
	resp, ok := body["adl.core.response"].(map[string]any)
	require.True(t, ok)
	evaluations, ok := resp["evaluations"].([]any)
	require.True(t, ok)
	require.Len(t, evaluations, 2)

	first, ok := evaluations[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, first["decision"])

	second, ok := evaluations[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, second["decision"])
}

func TestRecord_ResourceOmittedWhenUnconfigured(t *testing.T) {
	logger := &Logger{}

	rec := logger.record(testTraceContext(), EventAccessEvaluation, testEvaluationRequest(), nil, nil)

	assert.NotContains(t, marshaled(t, rec), "resource")
}

func TestRecord_ResourceIdentifiesProducerWhenConfigured(t *testing.T) {
	logger := &Logger{cfg: Config{Resource: map[string]string{testServiceKey: testServiceName}}}

	rec := logger.record(testTraceContext(), EventAccessEvaluation, testEvaluationRequest(), nil, nil)

	assert.Equal(t, map[string]any{testServiceKey: testServiceName}, marshaled(t, rec)["resource"])
}

func TestAuthZENResource_LiftsTypeAndIDFromTopazConvention(t *testing.T) {
	props, err := structpb.NewStruct(map[string]any{
		"object_type": "holiday-request",
		"object_id":   "446epbc8y7",
		"employee":    "bob",
	})
	require.NoError(t, err)

	resource := Config{}.AuthZENResource(props)

	assert.Equal(t, "holiday-request", resource.GetType())
	assert.Equal(t, "446epbc8y7", resource.GetId())
	assert.Equal(t, props, resource.GetProperties(), "the whole resource context is kept as properties")
}

func TestAuthZENResource_HonoursConfiguredKeys(t *testing.T) {
	props, err := structpb.NewStruct(map[string]any{testTypeKey: "document", testIDKey: "42"})
	require.NoError(t, err)

	cfg := Config{ResourceContext: ResourceContextConfig{TypeKey: testTypeKey, IDKey: testIDKey}}

	resource := cfg.AuthZENResource(props)

	assert.Equal(t, "document", resource.GetType())
	assert.Equal(t, "42", resource.GetId())
}

func TestAuthZENResource_NilPropertiesYieldEmptyResource(t *testing.T) {
	resource := Config{}.AuthZENResource(nil)

	assert.Empty(t, resource.GetType())
	assert.Empty(t, resource.GetId())
}
