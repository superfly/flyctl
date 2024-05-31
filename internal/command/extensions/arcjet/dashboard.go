package arcjet

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
		long = `Visit the Arcjet dashboard`

		short = long
		usage = "dashboard [site_name]"
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
		return extensions_core.OpenOrgDashboard(ctx, org, "arcjet")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeArcjet)

	if err != nil {
		return err
	}

	err = extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeArcjet)
	return
}
