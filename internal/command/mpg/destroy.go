package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
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
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunDestroy(ctx, cluster.Id)
	}

	return cmdv2.RunDestroy(ctx, cluster.Id)
}
