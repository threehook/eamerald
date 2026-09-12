package header

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/textproto"
	"strings"

	"google.golang.org/grpc/metadata"
)

var HeaderTraceParent = CtxKey(textproto.CanonicalMIMEHeaderKey("Traceparent"))

// TraceContext carries the W3C Trace Context IDs (hex-encoded) for a single
// request: https://www.w3.org/TR/trace-context/#traceparent-header.
type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
}

// ExtractTraceContext derives a TraceContext for the current gRPC request.
// If an incoming "traceparent" header is present and well-formed, its
// trace-id is reused and its parent-id becomes ParentSpanID. Otherwise a
// fresh trace-id is minted and ParentSpanID is left empty, marking this
// request as the root of a new trace. A fresh span-id is always minted.
func ExtractTraceContext(ctx context.Context) TraceContext {
	traceID, parentSpanID := "", ""

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(strings.ToLower(string(HeaderTraceParent))); len(values) > 0 {
			if tid, pid, ok := parseTraceParent(values[0]); ok {
				traceID, parentSpanID = tid, pid
			}
		}
	}

	if traceID == "" {
		traceID = randomHex(traceIDBytes)
	}

	return TraceContext{
		TraceID:      traceID,
		SpanID:       randomHex(spanIDBytes),
		ParentSpanID: parentSpanID,
	}
}

const (
	traceIDBytes          = 16
	spanIDBytes           = 8
	traceFlagsBytes       = 1
	traceParentPartsCount = 4
)

// parseTraceParent parses a "version-trace_id-parent_id-flags" traceparent
// value, accepting only the current version 00 format.
func parseTraceParent(v string) (string, string, bool) {
	parts := strings.Split(v, "-")
	if len(parts) != traceParentPartsCount {
		return "", "", false
	}

	version, traceID, parentID, flags := parts[0], parts[1], parts[2], parts[3]

	if version != "00" ||
		len(traceID) != hex.EncodedLen(traceIDBytes) ||
		len(parentID) != hex.EncodedLen(spanIDBytes) ||
		len(flags) != hex.EncodedLen(traceFlagsBytes) {
		return "", "", false
	}

	for _, s := range []string{traceID, parentID, flags} {
		if _, err := hex.DecodeString(s); err != nil {
			return "", "", false
		}
	}

	if traceID == strings.Repeat("0", hex.EncodedLen(traceIDBytes)) ||
		parentID == strings.Repeat("0", hex.EncodedLen(spanIDBytes)) {
		return "", "", false
	}

	return traceID, parentID, true
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}
