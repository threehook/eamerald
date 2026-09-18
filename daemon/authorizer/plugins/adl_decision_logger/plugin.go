package adl_decision_logger

import (
	"context"
	"io"
	"os"

	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/rs/zerolog"
)

const (
	PluginName = "adl_decision_logger"
	PluginDesc = "Logius ADL Level 1 Decision Logger"
)

type Plugin struct {
	manager *plugins.Manager
	config  *Config
	logger  *zerolog.Logger
	// out is where marshaled ADL records are written, one JSON line per
	// decision, when stdout output is active. It is a plain io.Writer, not a
	// zerolog logger: the line itself must be the ADL record with its fields
	// at the top level, not a record wrapped in zerolog's own
	// {"message": "..."} envelope. io.Discard when stdout output is off.
	out io.Writer
	// otel is non-nil only while otlp output is active and configured with
	// a reachable-looking endpoint. nil means "don't attempt OTLP export".
	otel *otelState
}

var _ plugins.Plugin = (*Plugin)(nil)

func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info().Bool("enabled", p.config.Enabled).Str("output", p.config.Output).Msg("start")

	outputs := p.config.outputs()

	if outputs[OutputStdout] {
		p.out = os.Stdout
	} else {
		p.out = io.Discard
	}

	if outputs[OutputOTLP] {
		p.startOTel(ctx)
	}

	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})

	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("started")

	return nil
}

func (p *Plugin) Stop(ctx context.Context) {
	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("stop")

	p.out = io.Discard

	if err := p.otel.shutdown(ctx); err != nil {
		p.logger.Error().Err(err).Msg("error shutting down otlp log export")
	}

	p.otel = nil

	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})

	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("stopped")
}

func (p *Plugin) Reconfigure(ctx context.Context, c any) {
	if cfg, ok := c.(*Config); ok {
		p.config = cfg
	}
}

// startOTel sets up OTLP export when configured. A missing endpoint or a
// setup error is logged and otlp output is simply skipped for this run,
// rather than failing Start and taking the rest of the plugin (including
// stdout output) down with it.
func (p *Plugin) startOTel(ctx context.Context) {
	if p.config.OTLP.Endpoint == "" {
		p.logger.Error().Msg("otlp output requested but opa.config.plugins.adl_decision_logger.otlp.endpoint is not set - skipping otlp output")
		return
	}

	state, err := newOTelState(ctx, p.config.OTLP)
	if err != nil {
		p.logger.Error().Err(err).Str("endpoint", p.config.OTLP.Endpoint).Msg("failed to set up otlp log export - skipping otlp output")
		return
	}

	p.otel = state
}

func Lookup(m *plugins.Manager) *Plugin {
	p := m.Plugin(PluginName)
	if p == nil {
		return nil
	}

	plugin, _ := p.(*Plugin)

	return plugin
}
