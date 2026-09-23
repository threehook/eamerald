package eds

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/threehook/eamerald/internal/adl"
	"github.com/threehook/eamerald/internal/eds/pkg/directory"
)

func New(
	ctx context.Context, config *directory.Config, logger *zerolog.Logger, adlLogger *adl.Logger,
) (*directory.Directory, error) {
	newLogger := logger.With().Str("component", "edge-ds").Logger()

	ds, err := directory.New(ctx, config, &newLogger, adlLogger)
	if err != nil {
		return nil, err
	}

	return ds, nil
}

// Open is New's non-singleton counterpart; see directory.Open.
func Open(
	ctx context.Context, config *directory.Config, logger *zerolog.Logger, adlLogger *adl.Logger,
) (*directory.Directory, error) {
	newLogger := logger.With().Str("component", "edge-ds").Logger()

	ds, err := directory.Open(ctx, config, &newLogger, adlLogger)
	if err != nil {
		return nil, err
	}

	return ds, nil
}
