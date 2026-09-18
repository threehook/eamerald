package adl_decision_logger

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/pkg/errors"
	"github.com/threehook/eamerald/internal/header"
)

// LogDecision writes a Logius ADL 1.0 Level 1 record for a Topaz Is() call,
// to every output configured via Config.Output (stdout and/or otlp - see
// config.go). The stdout line is the record itself (its fields at the top
// level), not wrapped in any other envelope, so a log-shipping agent can
// parse trace_id/event_name/status/body directly.
func (plugin *Plugin) LogDecision(ctx context.Context, req *authorizer.IsRequest, decisions []*authorizer.Decision) error {
	if !plugin.config.Enabled {
		return nil
	}

	tc := header.ExtractTraceContext(ctx)
	record := buildRecord(tc, req, decisions)

	return plugin.emit(ctx, tc, record)
}

// LogEvaluationError writes a status-Error ADL record for an Is() call that
// reached a resolved OPA runtime but failed before producing a decision
// (query preparation/validation, evaluation, or undefined results). For
// failures that occur before a runtime is resolved at all, there is no
// plugins.Manager to look a *Plugin up from - use LogEvaluationErrorDirect
// instead.
func (plugin *Plugin) LogEvaluationError(ctx context.Context, req *authorizer.IsRequest) error {
	if !plugin.config.Enabled {
		return nil
	}

	tc := header.ExtractTraceContext(ctx)
	record := buildErrorRecord(tc, req, req.GetPolicyContext().GetDecisions())

	return plugin.emit(ctx, tc, record)
}

// emit writes record to every output this Plugin instance has active. Errors
// from each output are joined, not short-circuited, so a broken otlp
// exporter doesn't also suppress the stdout copy (or vice versa).
func (plugin *Plugin) emit(ctx context.Context, tc header.TraceContext, record Record) error {
	var errs []error

	if err := writeRecordStdout(plugin.out, record); err != nil {
		errs = append(errs, err)
	}

	if plugin.otel != nil {
		if err := plugin.otel.emit(ctx, tc, record); err != nil {
			errs = append(errs, err)
		}
	}

	return stderrors.Join(errs...)
}

// LogEvaluationErrorDirect writes a status-Error ADL record without going
// through the OPA plugin manager, for Is() failures that occur before an
// OPA runtime is resolved (request validation, identity resolution, runtime
// lookup itself) - there is no *Plugin reachable at that point, since the
// plugin lives inside a runtime's plugins.Manager, so there is no already-
// initialized OTLP exporter to reuse either. Writes to stdout only, matching
// where a started *Plugin's stdout output goes. Callers are responsible for
// checking the plugin's enabled config first (see IsEnabled).
func LogEvaluationErrorDirect(ctx context.Context, req *authorizer.IsRequest) error {
	record := buildErrorRecord(header.ExtractTraceContext(ctx), req, req.GetPolicyContext().GetDecisions())

	return writeRecordStdout(os.Stdout, record)
}

// IsEnabled reports whether the adl_decision_logger plugin is enabled,
// read directly from raw OPA plugin config rather than a resolved runtime's
// plugin instance - usable before a runtime exists.
func IsEnabled(rawConfig any) bool {
	if rawConfig == nil {
		return false
	}

	b, err := json.Marshal(rawConfig)
	if err != nil {
		return false
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return false
	}

	return cfg.Enabled
}

func writeRecordStdout(out io.Writer, record Record) error {
	bytes, err := json.Marshal(record)
	if err != nil {
		return errors.Wrap(err, "error marshaling adl decision record")
	}

	bytes = append(bytes, '\n')

	if _, err := out.Write(bytes); err != nil {
		return errors.Wrap(err, "error writing adl decision record")
	}

	return nil
}
