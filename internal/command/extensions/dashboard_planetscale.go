package extensions

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newDashboardPlanetscale() (cmd *cobra.Command) {
	const (
		long = `View your PlanetScale database dashboard`

		short = long
		usage = "dashboard <database_name>"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	err = OpenDashboard(ctx, flag.FirstArg(ctx))
	return
}
