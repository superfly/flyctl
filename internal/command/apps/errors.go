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
		long = `Open the Sentry dashboard for the app's Sentry project in your browser,
using a single sign-on link from Fly.io. The app comes from --app or
fly.toml and must already have a Sentry project; create one with
` + "`fly ext sentry create`" + `.`
		short = `View application errors on Sentry.io`
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
