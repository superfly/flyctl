package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func newErrors() (cmd *cobra.Command) {
	const (
		long  = `View application errors on Sentry.io`
		short = long
		usage = "errors"
	)

	cmd = command.New(usage, short, long, RunDashboard, command.RequireSession, command.RequireAppName)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func RunDashboard(ctx context.Context) error {
	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeSentry)
	if err != nil {
		return err
	}

	return extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeSentry)
}
