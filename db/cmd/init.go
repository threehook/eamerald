package cmd

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/threehook/eamerald/internal/eds"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const requestTimeout = 5 * time.Second

func (cmd *InitCmd) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if !isPostgresDSN(cmd.Target) {
		if fi, err := os.Stat(cmd.Target); err == nil {
			if fi.IsDir() {
				return errors.Errorf("%s is a directory", cmd.Target)
			}

			return errors.Errorf("%s already exists", cmd.Target)
		}
	}

	logger := zerolog.New(io.Discard)

	dir, err := eds.New(ctx, configForTarget(cmd.Target), &logger, nil)
	if err != nil {
		log.Error().Err(err).Str("target", cmd.Target).Msg("init_cmd")
		return err
	}
	defer dir.Close()

	return nil
}
