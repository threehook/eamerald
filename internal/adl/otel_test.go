//nolint:testpackage // withSpanContext/newOTelState/otelState are unexported and only need to be verified from within the package.
package adl

import (
	"context"
	"testing"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threehook/eamerald/internal/header"
	"go.opentelemetry.io/otel/trace"
)

func TestWithSpanContext_ValidIDs_CarriesTraceAndSpanID(t *testing.T) {
	tc := testTraceContext()

	ctx := withSpanContext(t.Context(), tc)

	sc := trace.SpanContextFromContext(ctx)
	require.True(t, sc.IsValid())
	assert.Equal(t, tc.TraceID, sc.TraceID().String())
	assert.Equal(t, tc.SpanID, sc.SpanID().String())
}

func TestWithSpanContext_InvalidIDs_LeavesContextUnchanged(t *testing.T) {
	tc := header.TraceContext{
		TraceID: "not-valid-hex",
		SpanID:  "also-not-valid",
	}

	ctx := withSpanContext(t.Context(), tc)

	assert.False(t, trace.SpanContextFromContext(ctx).IsValid())
}

func TestNewOTelState_BuildsAndShutsDownCleanly(t *testing.T) {
	// No real collector needs to be listening: otlploggrpc.New dials lazily,
	// so construction and an immediate shutdown must both succeed without
	// ever needing a reachable endpoint.
	state, err := newOTelState(t.Context(), OTLPConfig{
		Endpoint: testOTLPEndpoint,
		Insecure: true,
	})
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.NoError(t, state.shutdown(context.Background()))
}

func TestOTelState_Shutdown_NilSafe(t *testing.T) {
	var state *otelState

	assert.NoError(t, state.shutdown(context.Background()))
}

func TestOTelState_Emit_DoesNotErrorWithoutCollector(t *testing.T) {
	// The batch processor queues locally and only attempts network I/O on
	// its own schedule or on Shutdown's forced flush - Emit itself must not
	// block on or fail from the collector being unreachable. (Shutdown is a
	// different story - see TestNewOTelState_BuildsAndShutsDownCleanly,
	// which shuts down with nothing queued; a flush with a record actually
	// queued legitimately times out against an unreachable collector, which
	// is exactly why Close logs that error rather than treating it as fatal.)
	state, err := newOTelState(t.Context(), OTLPConfig{
		Endpoint: testOTLPEndpoint,
		Insecure: true,
	})
	require.NoError(t, err)

	logger := &Logger{}
	tc := testTraceContext()
	record := logger.record(tc, EventAccessEvaluation, &dsa.EvaluationRequest{}, &dsa.EvaluationResponse{}, nil)

	assert.NoError(t, state.emit(t.Context(), tc, record))
}
