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
		short = "Visit the Wafris dashboard"
		long  = `Open the Wafris dashboard for a WAF in your browser, using a sign-in
link from Fly.io. Name the WAF as an argument, or run the command in an
app directory (or with ` + "`--app`" + `) to use the app's WAF. With ` + "`--org`" + `, or
with FLY_ORG set, it opens that organization's Wafris dashboard instead,
even if you name a WAF.

If you haven't accepted Wafris's terms of service yet, you're asked to;
` + "`--yes`" + ` accepts them, and is required to do so without a terminal.`

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
