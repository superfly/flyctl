package sentry_ext

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func dashboard() (cmd *cobra.Command) {
	const (
		long = `View Sentry issues for this application`

		short = long
		usage = "dashboard"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runDashboard(ctx context.Context) (err error) {

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeSentry)

	if err != nil {
		return err
	}

	err = extensions_core.OpenDashboard(ctx, extension.Name)
	return
}
