package cmd

import (
	"context"
	"io"
	"os"

	"github.com/threehook/eamerald/internal/eds/pkg/directory"
	"github.com/threehook/eamerald/topaz-db/pkg/inproc"
	dsc "github.com/threehook/eamerald/topaz/clients/directory"

	"github.com/rs/zerolog"
)

func (cmd *SetCmd) Run(ctx context.Context) error {
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

	var err error

	r := os.Stdin

	if cmd.Manifest != "" {
		r, err = os.Open(cmd.Manifest)
		if err != nil {
			return err
		}
	}

	return dsClient.SetManifest(ctx, r)
}
