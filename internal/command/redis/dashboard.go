package redis

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newDashboard() (cmd *cobra.Command) {
	const (
		long = `View your Upstash Redis databases on the Upstash web console`

		short = long
		usage = "dashboard <org_slug>"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	orgSlug := flag.FirstArg(ctx)

	result, err := gql.GetOrganization(ctx, client, orgSlug)
	if err != nil {
		return err
	}

	url := result.Organization.AddOnSsoLink
	fmt.Fprintf(io.Out, "Opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return
}
