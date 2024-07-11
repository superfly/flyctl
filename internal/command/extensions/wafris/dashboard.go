package wafris

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
		long = `Visit the Wafris dashboard`

		short = long
		usage = "dashboard [firewall_name]"
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
		return extensions_core.OpenOrgDashboard(ctx, org, "wafris")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeWafris)
	if err != nil {
		return err
	}

	return extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeWafris)
}
