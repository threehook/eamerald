package cmd

import (
	"context"
	"io"
	"path/filepath"

	"github.com/threehook/eamerald/db/pkg/inproc"
	"github.com/threehook/eamerald/internal/eds/pkg/directory"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"

	"github.com/rs/zerolog"
)

func (cmd *LoadCmd) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cfg := &directory.Config{
		DBPath:         cmd.DBFile,
		RequestTimeout: requestTimeout,
	}

	logger := zerolog.New(io.Discard)

	conn, cleanup := inproc.NewServer(ctx, &logger, cfg)
	defer cleanup()

	dsClient := dsc.New(conn)

	files, err := filepath.Glob(filepath.Join(cmd.DataDir, "*.json"))
	if err != nil {
		return err
	}

	return dsClient.Import(ctx, files)
}
