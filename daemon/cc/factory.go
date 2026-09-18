package cc

import (
	logger "github.com/aserto-dev/logger"
	"github.com/threehook/eamerald/daemon/cc/context"
	"github.com/threehook/eamerald/internal/certs"
	opalogger "github.com/threehook/eamerald/internal/runtime/logger"
	"github.com/threehook/eamerald/pkg/config"
)

// buildCC sets up the CC struct that contains all dependencies that are cross cutting.
func buildCC(
	logOutput logger.Writer,
	errOutput logger.ErrWriter,
	configPath config.Path,
	overrides config.Overrider,
) (
	*CC,
	func(),
	error,
) {
	errGroupAndContext := context.NewContext()
	contextContext := errGroupAndContext.Ctx

	loggerConfig, err := config.NewLoggerConfig(configPath, overrides)
	if err != nil {
		return nil, nil, err
	}

	zerologLogger, err := opalogger.NewLogger(logOutput, errOutput, loggerConfig)
	if err != nil {
		return nil, nil, err
	}

	ctx := zerologLogger.WithContext(errGroupAndContext.Ctx)

	generator := certs.NewGenerator(ctx)

	configConfig, err := config.NewConfig(configPath, zerologLogger, overrides, generator)
	if err != nil {
		return nil, nil, err
	}

	group := errGroupAndContext.ErrGroup
	ccCC := &CC{
		Context:  contextContext,
		Config:   configConfig,
		Log:      zerologLogger,
		ErrGroup: group,
	}

	return ccCC, func() {
	}, nil
}
