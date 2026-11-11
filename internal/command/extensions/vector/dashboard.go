package vector

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
		short = "Open the Upstash Vector dashboard in your browser"
		long  = `Open the Upstash web console for an app's Vector index in your browser,
using a sign-in link from Fly.io. Run the command in an app directory or
pass ` + "`--app`" + `. With ` + "`--org`" + `, it opens that organization's Upstash Vector
dashboard instead.

If you haven't accepted Upstash Vector's terms of service yet, you're
asked to; ` + "`--yes`" + ` accepts them.`

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
		return extensions_core.OpenOrgDashboard(ctx, org, "upstash_vector")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeUpstashVector)
	if err != nil {
		return err
	}

	return extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeUpstashVector)
}
