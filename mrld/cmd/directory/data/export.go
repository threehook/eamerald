package data

import (
	"context"
	"path/filepath"

	"github.com/threehook/eamerald/mrld/cc"
	"github.com/threehook/eamerald/mrld/clients"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"
)

type ExportCmd struct {
	dsc.Config

	Directory string `short:"d" required:"" help:"directory to write .json data" default:"${cwd}"`
}

func (cmd *ExportCmd) Run(ctx context.Context) error {
	if ok, err := clients.Validate(ctx, &cmd.Config); !ok {
		return err
	}

	cc.Con().Info().Msg(">>> exporting data to %s", cmd.Directory)

	objectsFile := filepath.Join(cmd.Directory, "objects.json")
	relationsFile := filepath.Join(cmd.Directory, "relations.json")

	dsClient, err := dsc.NewClient(ctx, &cmd.Config)
	if err != nil {
		return err
	}

	return dsClient.Export(ctx, objectsFile, relationsFile)
}
