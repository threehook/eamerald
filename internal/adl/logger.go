package adl

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/threehook/eamerald/internal/header"
)

// Logger writes one ADL record per authorization decision, to every output
// configured via Config.Output.
//
// There is one typed method per AuthZEN API. The endpoint that handled the
// decision picks the method, so event_name follows the call structurally and
// is never inferred from the shape of a request.
//
// A nil *Logger is a valid, disabled logger: call sites on the decision path
// do not need to nil-check before logging.
type Logger struct {
	cfg  Config
	log  *zerolog.Logger
	out  io.Writer
	otel *otelState
}

// New builds a Logger for cfg. A disabled logger, or one whose OTLP exporter
// could not be set up, still satisfies every call - an output that cannot be
// brought up is logged and skipped rather than failing startup and taking
// the remaining outputs down with it.
func New(ctx context.Context, cfg Config, log *zerolog.Logger) *Logger {
	newLog := log.With().Str("component", ConfigKey).Logger()

	logger := &Logger{cfg: cfg, log: &newLog, out: io.Discard}

	if !cfg.Enabled {
		return logger
	}

	outputs := cfg.outputs()

	if outputs[OutputStdout] {
		logger.out = os.Stdout
	}

	if outputs[OutputOTLP] {
		logger.startOTel(ctx)
	}

	newLog.Info().Str("output", cfg.Output).Msg("adl decision logging enabled")

	return logger
}

// Close flushes and shuts down any buffered output. Records still batched in
// the OTLP exporter would otherwise be dropped on exit.
func (l *Logger) Close(ctx context.Context) error {
	if l == nil {
		return nil
	}

	l.out = io.Discard

	err := l.otel.shutdown(ctx)
	l.otel = nil

	return err
}

// Config returns the configuration the logger was built from, so callers can
// reach the shared resource-context key mapping.
func (l *Logger) Config() Config {
	if l == nil {
		return Config{}
	}

	return l.cfg
}

// Evaluation logs a single access evaluation. A non-nil evalErr means the
// PDP could not produce a decision, and yields a status-Error record.
func (l *Logger) Evaluation(
	ctx context.Context, req *dsa.EvaluationRequest, resp *dsa.EvaluationResponse, evalErr error,
) error {
	return l.emit(ctx, EventAccessEvaluation, req, resp, evalErr)
}

// Evaluations logs a batch access evaluation. One call produces exactly one
// record, with the per-sub-decision outcomes carried in the response.
func (l *Logger) Evaluations(
	ctx context.Context, req *dsa.EvaluationsRequest, resp *dsa.EvaluationsResponse, evalErr error,
) error {
	return l.emit(ctx, EventAccessEvaluations, req, resp, evalErr)
}

func (l *Logger) SubjectSearch(
	ctx context.Context, req *dsa.SubjectSearchRequest, resp *dsa.SubjectSearchResponse, evalErr error,
) error {
	return l.emit(ctx, EventSearchSubject, req, resp, evalErr)
}

func (l *Logger) ResourceSearch(
	ctx context.Context, req *dsa.ResourceSearchRequest, resp *dsa.ResourceSearchResponse, evalErr error,
) error {
	return l.emit(ctx, EventSearchResource, req, resp, evalErr)
}

func (l *Logger) ActionSearch(
	ctx context.Context, req *dsa.ActionSearchRequest, resp *dsa.ActionSearchResponse, evalErr error,
) error {
	return l.emit(ctx, EventSearchAction, req, resp, evalErr)
}

func (l *Logger) active() bool {
	return l != nil && l.cfg.Enabled
}

// emit writes one record to every active output. Errors from each output are
// joined, not short-circuited, so a broken otlp exporter doesn't also
// suppress the stdout copy (or vice versa).
func (l *Logger) emit(ctx context.Context, eventName string, request, response any, evalErr error) error {
	if !l.active() {
		return nil
	}

	tc := header.ExtractTraceContext(ctx)
	rec := l.record(tc, eventName, request, response, evalErr)

	var errs []error

	if err := writeRecordStdout(l.out, rec); err != nil {
		errs = append(errs, err)
	}

	if l.otel != nil {
		if err := l.otel.emit(ctx, tc, rec); err != nil {
			errs = append(errs, err)
		}
	}

	return stderrors.Join(errs...)
}

func writeRecordStdout(out io.Writer, record Record) error {
	buf, err := json.Marshal(record)
	if err != nil {
		return errors.Wrap(err, "error marshaling adl decision record")
	}

	buf = append(buf, '\n')

	if _, err := out.Write(buf); err != nil {
		return errors.Wrap(err, "error writing adl decision record")
	}

	return nil
}
