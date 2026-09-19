//nolint:testpackage // the Logger is constructed with an in-memory writer, which is unexported.
package adl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger returns an enabled Logger writing to an in-memory buffer.
func testLogger(t *testing.T) (*Logger, *bytes.Buffer) {
	t.Helper()

	buf := &bytes.Buffer{}
	log := zerolog.New(io.Discard)

	return &Logger{cfg: Config{Enabled: true}, log: &log, out: buf}, buf
}

func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any

	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}

		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))

		out = append(out, m)
	}

	return out
}

// Each AuthZEN endpoint has its own typed method, so event_name follows the
// endpoint that handled the decision rather than being inferred from the
// shape of the request.
func TestLogger_EventNameFollowsEndpoint(t *testing.T) {
	cases := []struct {
		name      string
		eventName string
		log       func(context.Context, *Logger) error
	}{
		{"evaluation", EventAccessEvaluation, func(ctx context.Context, l *Logger) error {
			return l.Evaluation(ctx, &dsa.EvaluationRequest{}, &dsa.EvaluationResponse{}, nil)
		}},
		{"evaluations", EventAccessEvaluations, func(ctx context.Context, l *Logger) error {
			return l.Evaluations(ctx, &dsa.EvaluationsRequest{}, &dsa.EvaluationsResponse{}, nil)
		}},
		{"subject search", EventSearchSubject, func(ctx context.Context, l *Logger) error {
			return l.SubjectSearch(ctx, &dsa.SubjectSearchRequest{}, &dsa.SubjectSearchResponse{}, nil)
		}},
		{"resource search", EventSearchResource, func(ctx context.Context, l *Logger) error {
			return l.ResourceSearch(ctx, &dsa.ResourceSearchRequest{}, &dsa.ResourceSearchResponse{}, nil)
		}},
		{"action search", EventSearchAction, func(ctx context.Context, l *Logger) error {
			return l.ActionSearch(ctx, &dsa.ActionSearchRequest{}, &dsa.ActionSearchResponse{}, nil)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, buf := testLogger(t)

			require.NoError(t, tc.log(t.Context(), logger))

			written := records(t, buf)
			require.Len(t, written, 1, "one call must produce exactly one record")
			assert.Equal(t, tc.eventName, written[0]["event_name"])
		})
	}
}

// One API call yields one record however many sub-decisions it carries.
func TestLogger_EvaluationsWritesOneRecordForAllSubDecisions(t *testing.T) {
	logger, buf := testLogger(t)

	req := &dsa.EvaluationsRequest{
		Subject: &dsa.Subject{Type: "user", Id: "alice"},
		Evaluations: []*dsa.EvaluationRequest{
			{Action: &dsa.Action{Name: "read"}},
			{Action: &dsa.Action{Name: "write"}},
		},
	}
	resp := &dsa.EvaluationsResponse{
		Evaluations: []*dsa.EvaluationResponse{{Decision: true}, {Decision: false}},
	}

	require.NoError(t, logger.Evaluations(t.Context(), req, resp, nil))

	written := records(t, buf)
	require.Len(t, written, 1)

	body, ok := written[0]["body"].(map[string]any)
	require.True(t, ok)

	response, ok := body["adl.core.response"].(map[string]any)
	require.True(t, ok)

	evaluations, ok := response["evaluations"].([]any)
	require.True(t, ok)
	assert.Len(t, evaluations, 2, "per-sub-decision outcomes are carried in the single record's response")
}

func TestLogger_TraceContextIsCarried(t *testing.T) {
	logger, buf := testLogger(t)

	require.NoError(t, logger.Evaluation(t.Context(), &dsa.EvaluationRequest{}, &dsa.EvaluationResponse{}, nil))

	written := records(t, buf)
	require.Len(t, written, 1)

	traceID, ok := written[0]["trace_id"].(string)
	require.True(t, ok)
	assert.Len(t, traceID, 32, "trace_id is 16 bytes, hex encoded")

	spanID, ok := written[0]["span_id"].(string)
	require.True(t, ok)
	assert.Len(t, spanID, 16, "span_id is 8 bytes, hex encoded")
}

func TestLogger_DisabledWritesNothing(t *testing.T) {
	buf := &bytes.Buffer{}
	log := zerolog.New(io.Discard)
	logger := &Logger{cfg: Config{Enabled: false}, log: &log, out: buf}

	require.NoError(t, logger.Evaluation(t.Context(), &dsa.EvaluationRequest{}, &dsa.EvaluationResponse{}, nil))

	assert.Empty(t, buf.String())
}

// A nil *Logger is how services that do not log decisions (CLI tooling, the
// in-process directory) are wired, so every call has to tolerate it.
func TestLogger_NilIsSafe(t *testing.T) {
	var logger *Logger

	assert.NoError(t, logger.Evaluation(t.Context(), &dsa.EvaluationRequest{}, &dsa.EvaluationResponse{}, nil))
	assert.NoError(t, logger.ActionSearch(t.Context(), &dsa.ActionSearchRequest{}, &dsa.ActionSearchResponse{}, nil))
	assert.NoError(t, logger.Close(t.Context()))
	assert.Equal(t, Config{}, logger.Config())
}
