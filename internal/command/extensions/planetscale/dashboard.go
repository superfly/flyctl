package planetscale

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func dashboard() (cmd *cobra.Command) {
	const (
		long = `Visit the PlanetScale database dashboard`

		short = long
		usage = "dashboard <database_name>"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	err = extensions_core.OpenDashboard(ctx, flag.FirstArg(ctx))
	return
}
