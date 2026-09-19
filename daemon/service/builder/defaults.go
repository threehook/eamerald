package builder

import (
	"net/http"

	"github.com/go-http-utils/headers"
)

var DefaultGatewayAllowedHeaders = []string{
	headers.Authorization,
	headers.ContentType,
	headers.IfMatch,
	headers.IfNoneMatch,
	"Depth",
}

// TraceContextHeaders are the W3C Trace Context propagation headers. They are
// always forwarded to gRPC metadata, whatever allowed_headers is configured
// to, because the Authorization Decision Log has to record the trace context
// of the caller: a decision that arrives over the REST gateway without them
// would be logged under a freshly minted trace, breaking correlation with the
// transaction it belongs to.
//
// grpc-gateway's DefaultHeaderMatcher drops them - traceparent/tracestate are
// not IANA "permanent" message headers - so matching them has to be explicit.
var TraceContextHeaders = []string{"Traceparent", "Tracestate"}

var DefaultGatewayAllowedMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodHead,
	http.MethodDelete,
	http.MethodPut,
	http.MethodPatch,
	"PROPFIND",
	"MKCOL",
	"COPY",
	"MOVE",
}

var DefaultGatewayAllowedOrigins = []string{
	"http://localhost",
	"http://localhost:*",
	"https://localhost",
	"https://localhost:*",
	"http://127.0.0.1",
	"http://127.0.0.1:*",
	"https://127.0.0.1",
	"https://127.0.0.1:*",
}
