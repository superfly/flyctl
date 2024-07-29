package tigris

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
		long = `Visit the Tigris dashboard`

		short = long
		usage = "dashboard [bucket_name]"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,
	)
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func runDashboard(ctx context.Context) (err error) {

	org := flag.GetOrg(ctx)

	if org != "" {
		return extensions_core.OpenOrgDashboard(ctx, org, "tigris")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeTigris)

	if err != nil {
		return err
	}

	err = extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeTigris)
	return
}
