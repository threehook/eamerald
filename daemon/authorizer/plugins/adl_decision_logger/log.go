package adl_decision_logger

import (
	"context"
	"encoding/json"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2"
	"github.com/pkg/errors"
	"github.com/threehook/eamerald/internal/header"
)

// LogDecision writes a Logius ADL 1.0 Level 1 record for a Topaz Is() call
// as a single JSON line to stdout. The line is the record itself (its
// fields at the top level), not wrapped in any other envelope, so a
// log-shipping agent can parse trace_id/event_name/status/body directly.
func (plugin *Plugin) LogDecision(ctx context.Context, req *authorizer.IsRequest, decisions []*authorizer.Decision) error {
	if !plugin.config.Enabled {
		return nil
	}

	record := buildRecord(header.ExtractTraceContext(ctx), req, decisions)

	bytes, err := json.Marshal(record)
	if err != nil {
		return errors.Wrap(err, "error marshaling adl decision record")
	}

	bytes = append(bytes, '\n')

	if _, err := plugin.out.Write(bytes); err != nil {
		return errors.Wrap(err, "error writing adl decision record")
	}

	return nil
}
