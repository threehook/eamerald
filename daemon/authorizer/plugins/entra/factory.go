package entra

import (
	"bytes"
	"context"

	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	"github.com/go-viper/mapstructure/v2"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/util"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"github.com/threehook/eamerald/daemon/authorizer/plugins/noop"
	"google.golang.org/grpc"
)

type PluginFactory struct {
	ctx    context.Context
	logger *zerolog.Logger
	writer dsw.WriterClient
}

var _ plugins.Factory = (*PluginFactory)(nil)

func NewPluginFactory(ctx context.Context, logger *zerolog.Logger, dsConn *grpc.ClientConn) PluginFactory {
	return PluginFactory{
		ctx:    ctx,
		logger: logger,
		writer: dsw.NewWriterClient(dsConn),
	}
}

func (f PluginFactory) New(pm *plugins.Manager, config any) plugins.Plugin {
	cfg, ok := config.(*Config)
	if !ok {
		// panic as the plugins.Factory interface definition of New does not provide an error return,
		// nor does the OPA implementation handle nil plugins.
		// NOTE that Validate() is called before New, mitigating the risk of the panic occurring.
		panic("failed to parse entra directory sync plugin config")
	}

	if !cfg.Enabled {
		return &noop.Noop{
			Manager: pm,
			Name:    PluginName,
		}
	}

	return newEntraPlugin(f.logger, cfg, pm, f.writer)
}

func (PluginFactory) Validate(pm *plugins.Manager, config []byte) (any, error) {
	parsedConfig := Config{}

	v := viper.New()
	v.SetConfigType("json")

	if err := v.ReadConfig(bytes.NewReader(config)); err != nil {
		return nil, errors.Wrap(err, "error parsing entra directory sync config")
	}

	if err := v.UnmarshalExact(&parsedConfig, func(dc *mapstructure.DecoderConfig) { dc.TagName = "json" }); err != nil {
		return nil, errors.Wrap(err, "error parsing entra directory sync config")
	}

	return &parsedConfig, util.Unmarshal(config, &parsedConfig)
}
