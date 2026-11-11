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
		short = "Destroy a Managed Postgres cluster"
		long  = `Permanently destroy a Managed Postgres cluster and all of its data. This
can't be undone.

You're asked to confirm unless you pass --yes; without a terminal, --yes is
required. The cluster is scheduled for deletion and may take some time to
disappear. Find cluster IDs with ` + "`fly mpg list`" + `.`
		usage = "destroy <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runDestroy,
		command.RequireSession,
		requireMacaroonToken,
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
