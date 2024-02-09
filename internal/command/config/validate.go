package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newValidate() (cmd *cobra.Command) {
	const (
		short = "Validate an app's config file"
		long  = `Validates an application's config file against the Fly platform to
ensure it is correct and meaningful to the platform.`
	)
	cmd = command.New("validate", short, long, runValidate,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd, flag.App(), flag.AppConfig())
	return
}

func runValidate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	cfg := appconfig.ConfigFromContext(ctx)

	if err := cfg.SetMachinesPlatform(); err != nil {
		return err
	}
	err, extra_info := cfg.Validate(ctx)
	fmt.Fprintln(io.Out, extra_info)
	return err
}
