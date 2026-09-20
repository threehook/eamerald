package cmd

import (
	"github.com/threehook/eamerald/mrld/cmd/access"
	"github.com/threehook/eamerald/mrld/cmd/authorizer"
	"github.com/threehook/eamerald/mrld/cmd/certs"
	"github.com/threehook/eamerald/mrld/cmd/configure"
	"github.com/threehook/eamerald/mrld/cmd/directory"
	"github.com/threehook/eamerald/mrld/cmd/eamerald"
	"github.com/threehook/eamerald/mrld/cmd/templates"
)

type SaveContext bool

var Save SaveContext

//nolint:lll
type CLI struct {
	Start      eamerald.StartCmd        `cmd:"" help:"start eamerald instance (daemon mode)"`
	Stop       eamerald.StopCmd         `cmd:"" help:"stop eamerald instance"`
	Restart    eamerald.RestartCmd      `cmd:"" help:"restart eamerald instance"`
	Status     eamerald.StatusCmd       `cmd:"" help:"status of eamerald daemon process"`
	Config     configure.ConfigCmd      `cmd:"" help:"configure eamerald instance"`
	Run        eamerald.RunCmd          `cmd:"" help:"start eamerald instance (console mode)"`
	Templates  templates.TemplateCmd    `cmd:"" help:"template commands"`
	Console    eamerald.ConsoleCmd      `cmd:"" help:"open eamerald console in the browser"`
	Directory  directory.DirectoryCmd   `cmd:"" aliases:"ds" help:"directory service commands"`
	Authorizer authorizer.AuthorizerCmd `cmd:"" aliases:"az" help:"authorizer service commands"`
	Access     access.AccessCmd         `cmd:"" aliases:"ac" help:"access service commands "`
	Certs      certs.CertsCmd           `cmd:"" help:"certificate management"`
	Install    eamerald.InstallCmd      `cmd:"" help:"install eamerald container"`
	Uninstall  eamerald.UninstallCmd    `cmd:"" help:"uninstall eamerald container"`
	Update     eamerald.UpdateCmd       `cmd:"" help:"update eamerald container version"`
	Version    VersionCmd               `cmd:"" help:"version information"`
	NoCheck    bool                     `flag:"" name:"no-check" json:"no_check,omitempty" short:"N" env:"EAMERALD_NO_CHECK" help:"disable local container status check"`
	NoColor    bool                     `flag:"" name:"no-color" json:"no_color,omitempty" env:"EAMERALD_NO_COLOR" help:"disable colored terminal output"`
	LogLevel   int                      `flag:"" name:"verbosity" short:"v" type:"counter" default:"0" help:"log level"`
}
