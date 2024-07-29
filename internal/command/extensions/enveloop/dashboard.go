package enveloop

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
		long = `Open the Enveloop dashboard via your web browser`

		short = long
		usage = "dashboard"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	if org := flag.GetOrg(ctx); org != "" {
		return extensions_core.OpenOrgDashboard(ctx, org, "enveloop")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeEnveloop)
	if err != nil {
		return err
	}
	return extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeEnveloop)
}
