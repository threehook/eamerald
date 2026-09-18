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
	// decision. It is a plain io.Writer, not a zerolog logger: the line
	// itself must be the ADL record with its fields at the top level, not a
	// record wrapped in zerolog's own {"message": "..."} envelope.
	out io.Writer
}

var _ plugins.Plugin = (*Plugin)(nil)

func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("start")

	p.out = os.Stdout

	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})

	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("started")

	return nil
}

func (p *Plugin) Stop(ctx context.Context) {
	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("stop")

	p.out = io.Discard

	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})

	p.logger.Info().Bool("enabled", p.config.Enabled).Msg("stopped")
}

func (p *Plugin) Reconfigure(ctx context.Context, c any) {
	if cfg, ok := c.(*Config); ok {
		p.config = cfg
	}
}

func Lookup(m *plugins.Manager) *Plugin {
	p := m.Plugin(PluginName)
	if p == nil {
		return nil
	}

	plugin, _ := p.(*Plugin)

	return plugin
}
