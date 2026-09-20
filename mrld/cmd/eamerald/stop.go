package eamerald

import (
	"context"
	"fmt"
	"os"

	"github.com/threehook/eamerald/mrld/cc"
	"github.com/threehook/eamerald/mrld/cmd/common"
	"github.com/threehook/eamerald/mrld/dockerx"
	"github.com/threehook/eamerald/pkg/config"
)

type StopCmd struct {
	ContainerName string `optional:"" default:"${container_name}" env:"CONTAINER_NAME" help:"container name"`
	Wait          bool   `optional:"" default:"false" help:"wait for ports to be closed"`
}

func (cmd *StopCmd) Run(ctx context.Context) error {
	dc, err := dockerx.New()
	if err != nil {
		return err
	}

	c := cc.GetConfig()

	c.Defaults.NoCheck = false // enforce that Stop does not bypass CheckRunStatus() to short-circuit.
	if c.CheckRunStatus(cmd.ContainerName, cc.StatusNotRunning) {
		return nil
	}

	cc.Con().Info().Msg(">>> stopping topaz %q...", c.Running.Config)

	if err := dc.Stop(cmd.ContainerName); err != nil {
		return err
	}

	if cmd.Wait {
		ports, err := config.GetConfig(c.Running.ConfigFile).Ports()
		if err != nil {
			return err
		}

		if err := cc.WaitForPorts(ports, cc.PortClosed); err != nil {
			return err
		}
	}

	// empty running config
	c.Running = cc.RunningConfig{}

	if err := c.SaveContextConfig(common.CLIConfigurationFile); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	return nil
}
