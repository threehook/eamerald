package configure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/threehook/eamerald/mrld/cc"
	"github.com/threehook/eamerald/mrld/cmd/common"
	"github.com/threehook/eamerald/mrld/editor"
)

type EditConfigCmd struct {
	Name      ConfigName `arg:"" required:"" default:"${active_config}" help:"topaz config name"`
	ConfigDir string     `flag:"" default:"${eamerald_cfg_dir}" help:"path to config folder"`
}

func (cmd *EditConfigCmd) Run(ctx context.Context) error {
	cfg := filepath.Join(cmd.ConfigDir, fmt.Sprintf("%s.yaml", cmd.Name))
	if cmd.Name == "defaults" {
		cfg = filepath.Join(cc.GetEameraldDir(), common.CLIConfigurationFile)
	}

	if _, err := os.Stat(cfg); err != nil {
		return err
	}

	e := editor.NewDefaultEditor([]string{"EAMERALD_EDITOR", "EDITOR"})

	if err := e.Launch(cfg); err != nil {
		return err
	}

	return nil
}
