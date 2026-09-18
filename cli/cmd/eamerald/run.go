package eamerald

import (
	"context"

	"github.com/threehook/eamerald/cli/cc"
)

type RunCmd struct {
	StartRunCmd
}

func (cmd *RunCmd) Run(ctx context.Context, cfg *cc.Config) error {
	return cmd.run(cfg, modeInteractive)
}
