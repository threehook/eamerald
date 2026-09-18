package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/threehook/eamerald/cli/cc"
	"github.com/threehook/eamerald/cli/x"
	"github.com/threehook/eamerald/db/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli := cmd.CLI{}

	kongCtx := kong.Parse(&cli,
		kong.Name("eamerald-db"),
		kong.Description("eamerald database utility"),
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
			"directory_svc":   os.Getenv(x.EnvEameraldDirectorySvc),
			"directory_key":   os.Getenv(x.EnvEameraldDirectoryKey),
			"directory_token": "",
			"insecure":        strconv.FormatBool(false),
			"plaintext":       strconv.FormatBool(false),
			"no_check":        strconv.FormatBool(false),
			"timeout":         cc.Timeout().String(),
		},
	)

	kongCtx.BindTo(ctx, (*context.Context)(nil))

	if err := kongCtx.Run(); err != nil {
		kongCtx.FatalIfErrorf(err)
	}
}
