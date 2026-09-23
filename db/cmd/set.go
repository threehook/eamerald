package cmd

import (
	"context"
	"io"
	"os"

	"github.com/threehook/eamerald/db/pkg/inproc"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"

	"github.com/rs/zerolog"
)

func (cmd *SetCmd) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := zerolog.New(io.Discard)

	conn, cleanup := inproc.NewServer(ctx, &logger, configForTarget(cmd.Target))
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
