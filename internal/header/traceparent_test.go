package header_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threehook/eamerald/internal/header"
	"google.golang.org/grpc/metadata"
)

func TestExtractTraceContext_NoIncomingHeader(t *testing.T) {
	tc := header.ExtractTraceContext(context.Background())

	assert.Len(t, tc.TraceID, 32)
	assert.Len(t, tc.SpanID, 16)
	assert.Empty(t, tc.ParentSpanID)
}

func TestExtractTraceContext_ValidTraceParent(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	))

	tc := header.ExtractTraceContext(ctx)

	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", tc.TraceID)
	assert.Equal(t, "00f067aa0ba902b7", tc.ParentSpanID)
	assert.Len(t, tc.SpanID, 16)
	assert.NotEqual(t, tc.ParentSpanID, tc.SpanID, "a fresh span-id must be minted, not reused from the header")
}

func TestExtractTraceContext_MalformedTraceParent(t *testing.T) {
	cases := []string{
		"not-a-traceparent",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // unsupported version
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // all-zero trace-id is invalid
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", // all-zero parent-id is invalid
		"00-tooshort-00f067aa0ba902b7-01",
	}

	for _, v := range cases {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("traceparent", v))

		tc := header.ExtractTraceContext(ctx)

		assert.Len(t, tc.TraceID, 32, "should fall back to a freshly minted trace-id for %q", v)
		assert.Empty(t, tc.ParentSpanID, "should not surface a parent span for malformed header %q", v)
	}
}
