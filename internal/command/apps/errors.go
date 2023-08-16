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
		long = `View application errors on Sentry.io`

		short = long
		usage = "errors"
	)

	cmd = command.New(usage, short, long, RunDashboard, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func RunDashboard(ctx context.Context) (err error) {

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeSentry)

	if err != nil {
		return err
	}

	err = extensions_core.OpenDashboard(ctx, extension.Name)
	return
}
