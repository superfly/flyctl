package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy a managed Postgres cluster"
		long  = short + ". " +
			`This command will permanently destroy a managed Postgres cluster and all its data.
This action is not reversible.`
		usage = "destroy <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runDestroy,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"delete", "remove", "rm"}

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	clusterId := flag.FirstArg(ctx)

	return cmdv1.RunDestroy(ctx, clusterId)
}
