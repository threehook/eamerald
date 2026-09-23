package cmd

import (
	"context"
	"io"
	"path/filepath"

	"github.com/threehook/eamerald/db/pkg/inproc"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"

	"github.com/rs/zerolog"
)

func (cmd *LoadCmd) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := zerolog.New(io.Discard)

	conn, cleanup := inproc.NewServer(ctx, &logger, configForTarget(cmd.Target))
	defer cleanup()

	dsClient := dsc.New(conn)

	files, err := filepath.Glob(filepath.Join(cmd.DataDir, "*.json"))
	if err != nil {
		return err
	}

	return dsClient.Import(ctx, files)
}
