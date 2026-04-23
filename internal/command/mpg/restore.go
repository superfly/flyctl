package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
)

func newRestore() *cobra.Command {
	const (
		long  = `Restore a Managed Postgres cluster from a backup.`
		short = "Restore MPG cluster from backup."
		usage = "restore <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runRestore,
		command.RequireSession,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "backup-id",
			Description: "The backup ID to restore from",
		},
	)

	return cmd
}

func runRestore(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunRestore(ctx, clusterID)
}
