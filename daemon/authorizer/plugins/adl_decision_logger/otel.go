package adl_decision_logger

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/threehook/eamerald/internal/header"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
)

// otelState holds the OTel logs SDK objects needed to emit and, on Stop,
// flush and close. Kept separate from *Plugin's other fields so Start/Stop
// only touch this when otlp output is actually active.
type otelState struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

// newOTelState builds an OTLP gRPC log exporter and a batching LoggerProvider
// for it. The batch processor makes emission asynchronous: LogDecision/
// LogEvaluationError never block the Is() hot path on network I/O to the
// collector, and a collector outage cannot fail authorization requests.
func newOTelState(ctx context.Context, cfg OTLPConfig) (*otelState, error) {
	exporterOpts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		exporterOpts = append(exporterOpts, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "error creating otlp log exporter")
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	return &otelState{
		provider: provider,
		logger:   provider.Logger(PluginName),
	}, nil
}

// shutdown flushes any batched records and closes the exporter connection.
// Must be called on plugin Stop so records buffered at shutdown are not
// silently dropped.
func (s *otelState) shutdown(ctx context.Context) error {
	if s == nil || s.provider == nil {
		return nil
	}

	return s.provider.Shutdown(ctx)
}

// emit sends record via OTLP, carrying the exact same JSON body as the
// stdout output and native trace/span correlation derived from tc.
func (s *otelState) emit(ctx context.Context, tc header.TraceContext, record Record) error {
	body, err := json.Marshal(record)
	if err != nil {
		return errors.Wrap(err, "error marshaling adl decision record for otlp")
	}

	otelRecord := otellog.Record{}
	otelRecord.SetTimestamp(time.UnixMilli(record.Timestamp))
	otelRecord.SetEventName(record.EventName)
	otelRecord.SetBody(attribute.StringValue(string(body)))

	if record.Status == StatusError {
		otelRecord.SetSeverity(otellog.SeverityError)
	} else {
		otelRecord.SetSeverity(otellog.SeverityInfo)
	}

	s.logger.Emit(withSpanContext(ctx, tc), otelRecord)

	return nil
}

// withSpanContext threads tc's trace/span IDs into ctx as an OTel
// SpanContext, so the logs SDK stamps the emitted record's trace_id/span_id
// from it. A malformed ID (should not happen - both come from
// header.ExtractTraceContext, which always mints valid hex) simply leaves
// the record without correlation rather than erroring the whole emit.
func withSpanContext(ctx context.Context, tc header.TraceContext) context.Context {
	traceID, err := trace.TraceIDFromHex(tc.TraceID)
	if err != nil {
		return ctx
	}

	spanID, err := trace.SpanIDFromHex(tc.SpanID)
	if err != nil {
		return ctx
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	})

	return trace.ContextWithSpanContext(ctx, sc)
}
