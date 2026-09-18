package configure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/threehook/eamerald/cli/cc"
	"github.com/threehook/eamerald/cli/cmd/common"
)

type DeleteConfigCmd struct {
	Name      ConfigName `arg:"" required:"" help:"topaz config name"`
	ConfigDir string     `flag:"" required:"false" default:"${eamerald_cfg_dir}" help:"path to config folder" `
}

func (cmd *DeleteConfigCmd) Run(ctx context.Context) error {
	cfg := cc.GetConfig()

	if cfg.CheckRunStatus(cc.ContainerName(fmt.Sprintf("%s.yaml", cmd.Name)), cc.StatusRunning) {
		return errors.Errorf("configuration %q is running, use 'topaz stop' to stop, before deleting", cmd.Name)
	}

	cc.Con().Info().Msg("Removing configuration %q", cmd.Name)

	filename := filepath.Join(cmd.ConfigDir, fmt.Sprintf("%s.yaml", cmd.Name))

	if cfg.Active.Config == cmd.Name.String() {
		cfg.Active.Config = ""
		cfg.Active.ConfigFile = ""

		if err := cfg.SaveContextConfig(common.CLIConfigurationFile); err != nil {
			return errors.Wrap(err, "failed to update active context")
		}
	}

	return os.Remove(filename)
}
