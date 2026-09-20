package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"github.com/threehook/eamerald/mrld/cc"
	"github.com/threehook/eamerald/mrld/cmd"
	"github.com/threehook/eamerald/mrld/cmd/common"
	"github.com/threehook/eamerald/mrld/constants"
	"github.com/threehook/eamerald/mrld/fflag"

	ver "github.com/threehook/eamerald/mrld/version"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
)

const (
	rcOK  int = 0
	rcErr int = 1
)

func main() {
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "--help")
	}

	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fflag.Init()

	cli := cmd.CLI{}

	cliConfigFile := filepath.Join(cc.GetEameraldDir(), common.CLIConfigurationFile)

	c, err := cc.NewConfig(ctx, cli.NoCheck, cliConfigFile)
	if err != nil {
		return exitErr(err)
	}

	if err := checkVersion(c); err != nil {
		return exitErr(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return exitErr(errors.Wrap(err, "failed to determine current working directory"))
	}

	kongCtx := kongParse(c, &cli, cwd)

	zerolog.SetGlobalLevel(logLevel(cli.LogLevel))

	if cli.NoColor {
		_ = os.Setenv(constants.EnvEameraldNoColor, strconv.FormatBool(true))
	}

	if err := cc.EnsureDirs(); err != nil {
		return exitErr(err)
	}

	kongCtx.BindTo(ctx, (*context.Context)(nil))

	if err := kongCtx.Run(c); err != nil {
		return exitErr(err)
	}

	return rcOK
}

func kongParse(cfg *cc.Config, cli *cmd.CLI, cwd string) *kong.Context {
	kongCtx := kong.Parse(cli,
		kong.Name(constants.AppName),
		kong.Description(constants.AppDescription),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			NoAppSummary:        false,
			Summary:             false,
			Compact:             true,
			Tree:                false,
			FlagsLast:           true,
			Indenter:            kong.SpaceIndenter,
			NoExpandSubcommands: true,
		}),
		kong.Vars{
			"eamerald_dir":           cc.GetEameraldDir(),
			"eamerald_certs_dir":     cc.GetEameraldCertsDir(),
			"eamerald_cfg_dir":       cc.GetEameraldCfgDir(),
			"eamerald_db_dir":        cc.GetEameraldDataDir(),
			"eamerald_decisions_dir": cc.GetEameraldDecisionsDir(),
			"eamerald_tmpl_dir":      cc.GetEameraldTemplateDir(),
			"eamerald_tmpl_url":      cc.GetEameraldTemplateURL(),
			"container_registry":     cc.ContainerRegistry(),
			"container_image":        cc.ContainerImage(),
			"container_tag":          cc.ContainerTag(),
			"container_platform":     cc.ContainerPlatform(),
			"container_name":         cc.ContainerName(cfg.Active.ConfigFile),
			"directory_svc":          cc.DirectorySvc(),
			"directory_key":          cc.DirectoryKey(),
			"directory_token":        cc.DirectoryToken(),
			"authorizer_svc":         cc.AuthorizerSvc(),
			"authorizer_key":         cc.AuthorizerKey(),
			"authorizer_token":       cc.AuthorizerToken(),
			"insecure":               strconv.FormatBool(cc.Insecure()),
			"plaintext":              strconv.FormatBool(cc.Plaintext()),
			"no_check":               strconv.FormatBool(cc.NoCheck()),
			"no_color":               strconv.FormatBool(cc.NoColor()),
			"active_config":          cfg.Active.Config,
			"cwd":                    cwd,
			"timeout":                cc.Timeout().String(),
		},
	)

	return kongCtx
}

func exitErr(err error) int {
	fmt.Fprintln(os.Stderr, err.Error())
	return rcErr
}

const (
	logLevelDisabled int = iota
	logLevelInfo
	logLevelWarn
	logLevelError
	logLevelDebug
	logLevelTrace
)

func logLevel(level int) zerolog.Level {
	switch level {
	case logLevelDisabled:
		return zerolog.Disabled
	case logLevelInfo:
		return zerolog.InfoLevel
	case logLevelWarn:
		return zerolog.WarnLevel
	case logLevelError:
		return zerolog.ErrorLevel
	case logLevelDebug:
		return zerolog.DebugLevel
	case logLevelTrace:
		return zerolog.TraceLevel
	default:
		return zerolog.Disabled
	}
}

// check set version in defaults and suggest update if needed.
func checkVersion(cfg *cc.Config) error {
	if cc.ContainerTag() == "latest" {
		return nil
	}

	buildVer, err := semver.NewVersion(ver.GetInfo().Version)
	if err != nil {
		return err
	}

	if buildVer.Prerelease() != "" {
		return nil
	}

	tagVer, err := semver.NewVersion(cc.ContainerTag())
	if err != nil {
		return err
	}

	if buildVer.Major() == tagVer.Major() && buildVer.Minor() == tagVer.Minor() && buildVer.Patch() == tagVer.Patch() {
		return nil
	}

	cc.Con().Warn().Msg("The default container tag configuration setting (%s), is different from the current topaz version (%v).",
		cfg.Defaults.ContainerTag,
		ver.GetInfo().Version,
	)

	if !common.PromptYesNo("Do you want to update the configuration setting?", false) {
		return nil
	}

	cfg.Defaults.ContainerTag = ver.GetInfo().Version

	return cfg.SaveContextConfig(common.CLIConfigurationFile)
}
