//nolint:testpackage // incomingHeaderMatcher is unexported and only needs to be verified from within the package.
package builder

import (
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The Authorization Decision Log has to record the trace context of the
// caller, so traceparent/tracestate must reach gRPC metadata even though
// grpc-gateway's default matcher drops them: they are not IANA permanent
// message headers, so without an explicit match a decision delivered over
// REST would be logged under a freshly minted trace.
func TestIncomingHeaderMatcher_ForwardsTraceContext(t *testing.T) {
	// The narrowest configuration the shipped config templates produce.
	matcher := incomingHeaderMatcher(append([]string{"Authorization", "Content-Type"}, TraceContextHeaders...))

	for _, header := range TraceContextHeaders {
		key, ok := matcher(textproto.CanonicalMIMEHeaderKey(header))

		assert.True(t, ok, "%s must be forwarded to gRPC metadata", header)
		assert.Equal(t, header, key, "%s must be forwarded under its own name, not prefixed", header)
	}
}

func TestIncomingHeaderMatcher_TraceContextIsDroppedWithoutExplicitMatch(t *testing.T) {
	// Guards the reason the explicit match above is needed at all: the
	// grpc-gateway fallback alone does not carry trace context.
	matcher := incomingHeaderMatcher([]string{"Authorization"})

	_, ok := matcher("Traceparent")

	assert.False(t, ok)
}

func TestIncomingHeaderMatcher_FallsBackToDefaultForPermanentHeaders(t *testing.T) {
	matcher := incomingHeaderMatcher(nil)

	key, ok := matcher("Accept")

	assert.True(t, ok)
	assert.Equal(t, "grpcgateway-Accept", key)
}
