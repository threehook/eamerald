//nolint:testpackage // buildRecord is unexported and only needs to be verified from within the package.
package adl_decision_logger

import (
	"encoding/json"
	"testing"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threehook/eamerald/internal/header"
)

const (
	decisionAllowed = "allowed"
	decisionEnabled = "enabled"
)

func testTraceContext() header.TraceContext {
	return header.TraceContext{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "",
	}
}

func testRequest() *authorizer.IsRequest {
	return &authorizer.IsRequest{
		IdentityContext: &api.IdentityContext{
			Type:     api.IdentityType_IDENTITY_TYPE_SUB,
			Identity: "alice",
		},
	}
}

func TestBuildRecord_SingleDecision_UsesSingularEventName(t *testing.T) {
	decisions := []*authorizer.Decision{
		{Decision: decisionAllowed, Is: true},
	}

	record := buildRecord(testTraceContext(), testRequest(), decisions)

	assert.Equal(t, EventNameAccessEvaluation, record.EventName)
	assert.Equal(t, StatusOk, record.Status)

	req, ok := record.Body.Request.(*dsa.EvaluationRequest)
	require.True(t, ok, "expected a singular EvaluationRequest")
	assert.Equal(t, decisionAllowed, req.GetAction().GetName())
	assert.Equal(t, "alice", req.GetSubject().GetId())

	resp, ok := record.Body.Response.(*dsa.EvaluationResponse)
	require.True(t, ok, "expected a singular EvaluationResponse")
	assert.True(t, resp.GetDecision())
}

func TestBuildRecord_MultipleDecisions_UsesBatchEventName(t *testing.T) {
	decisions := []*authorizer.Decision{
		{Decision: decisionAllowed, Is: true},
		{Decision: decisionEnabled, Is: false},
	}

	record := buildRecord(testTraceContext(), testRequest(), decisions)

	assert.Equal(t, EventNameAccessEvaluations, record.EventName)

	req, ok := record.Body.Request.(*dsa.EvaluationsRequest)
	require.True(t, ok, "expected a batch EvaluationsRequest")
	require.Len(t, req.GetEvaluations(), 2)
	assert.Equal(t, decisionAllowed, req.GetEvaluations()[0].GetAction().GetName())
	assert.Equal(t, decisionEnabled, req.GetEvaluations()[1].GetAction().GetName())

	resp, ok := record.Body.Response.(*dsa.EvaluationsResponse)
	require.True(t, ok, "expected a batch EvaluationsResponse")
	require.Len(t, resp.GetEvaluations(), 2)
	assert.True(t, resp.GetEvaluations()[0].GetDecision())
	assert.False(t, resp.GetEvaluations()[1].GetDecision())
}

func TestBuildRecord_JSONShape(t *testing.T) {
	record := buildRecord(testTraceContext(), testRequest(), []*authorizer.Decision{
		{Decision: decisionAllowed, Is: true},
	})

	b, err := json.Marshal(record)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	for _, field := range []string{"trace_id", "span_id", "event_name", "timestamp", "status", "attributes", "body"} {
		assert.Contains(t, m, field)
	}

	assert.NotContains(t, m, "parent_span_id", "empty parent_span_id must be omitted, not emitted as an empty string")

	body, ok := m["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "adl.core.request")
	assert.Contains(t, body, "adl.core.response")
}

func TestBuildRecord_ParentSpanIDPresentWhenNotTraceRoot(t *testing.T) {
	tc := testTraceContext()
	tc.ParentSpanID = "00f067aa0ba902b7"

	record := buildRecord(tc, testRequest(), []*authorizer.Decision{
		{Decision: decisionAllowed, Is: true},
	})

	b, err := json.Marshal(record)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	assert.Equal(t, "00f067aa0ba902b7", m["parent_span_id"])
}

func TestBuildErrorRecord_StatusIsError(t *testing.T) {
	record := buildErrorRecord(testTraceContext(), testRequest(), []string{decisionAllowed})

	assert.Equal(t, StatusError, record.Status)
}

func TestBuildErrorRecord_ResponseOmitted(t *testing.T) {
	// §3.3.8: response MAY be omitted when status is Error - the PDP never
	// reached a decision, so there is nothing to report.
	record := buildErrorRecord(testTraceContext(), testRequest(), []string{decisionAllowed})

	b, err := json.Marshal(record)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	body, ok := m["body"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "adl.core.request")
	assert.NotContains(t, body, "adl.core.response")
}

func TestBuildErrorRecord_SingleDecision_UsesSingularEventName(t *testing.T) {
	record := buildErrorRecord(testTraceContext(), testRequest(), []string{decisionAllowed})

	assert.Equal(t, EventNameAccessEvaluation, record.EventName)

	req, ok := record.Body.Request.(*dsa.EvaluationRequest)
	require.True(t, ok, "expected a singular EvaluationRequest")
	assert.Equal(t, decisionAllowed, req.GetAction().GetName())
}

func TestBuildErrorRecord_MultipleDecisions_UsesBatchEventName(t *testing.T) {
	record := buildErrorRecord(testTraceContext(), testRequest(), []string{decisionAllowed, decisionEnabled})

	assert.Equal(t, EventNameAccessEvaluations, record.EventName)

	req, ok := record.Body.Request.(*dsa.EvaluationsRequest)
	require.True(t, ok, "expected a batch EvaluationsRequest")
	require.Len(t, req.GetEvaluations(), 2)
}

func TestBuildErrorRecord_NoDecisions_UsesBatchEventNameWithNoEvaluations(t *testing.T) {
	// A request can fail validation before any decisions were even parsed
	// (e.g. isVerifyRequest rejecting an empty decisions list) - must still
	// produce a valid record, not panic or pick an arbitrary shape.
	record := buildErrorRecord(testTraceContext(), testRequest(), nil)

	assert.Equal(t, EventNameAccessEvaluations, record.EventName)

	req, ok := record.Body.Request.(*dsa.EvaluationsRequest)
	require.True(t, ok, "expected a batch EvaluationsRequest")
	assert.Empty(t, req.GetEvaluations())
}

func TestIsEnabled(t *testing.T) {
	assert.False(t, IsEnabled(nil))
	assert.False(t, IsEnabled(map[string]any{decisionEnabled: false}))
	assert.True(t, IsEnabled(map[string]any{decisionEnabled: true}))
	assert.False(t, IsEnabled("not a config object"))
}
