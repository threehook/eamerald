package git

import (
	"bytes"
	"context"

	"github.com/aserto-dev/topaz/topazd/authorizer/plugins/noop"
	"github.com/go-viper/mapstructure/v2"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/util"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type PluginFactory struct {
	ctx    context.Context
	logger *zerolog.Logger
}

var _ plugins.Factory = (*PluginFactory)(nil)

func NewPluginFactory(ctx context.Context, logger *zerolog.Logger) PluginFactory {
	return PluginFactory{
		ctx:    ctx,
		logger: logger,
	}
}

func (f PluginFactory) New(pm *plugins.Manager, config any) plugins.Plugin {
	cfg, ok := config.(*Config)
	if !ok {
		// panic as the plugins.Factory interface definition of New does not provide an error return,
		// nor does the OPA implementation handle nil plugins.
		// NOTE that Validate() is called before New, mitigating the risk of the panic occurring.
		panic("failed to parse git policy source plugin config")
	}

	if !cfg.Enabled {
		return &noop.Noop{
			Manager: pm,
			Name:    PluginName,
		}
	}

	return newGitPlugin(f.logger, cfg, pm)
}

func (PluginFactory) Validate(pm *plugins.Manager, config []byte) (any, error) {
	parsedConfig := Config{}

	v := viper.New()
	v.SetConfigType("json")

	if err := v.ReadConfig(bytes.NewReader(config)); err != nil {
		return nil, errors.Wrap(err, "error parsing git policy source config")
	}

	if err := v.UnmarshalExact(&parsedConfig, func(dc *mapstructure.DecoderConfig) { dc.TagName = "json" }); err != nil {
		return nil, errors.Wrap(err, "error parsing git policy source config")
	}

	return &parsedConfig, util.Unmarshal(config, &parsedConfig)
}
