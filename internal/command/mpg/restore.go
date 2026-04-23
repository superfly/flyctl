package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/mpg/utils"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
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
	out := iostreams.FromContext(ctx).Out

	var cluster *utils.ManagedCluster
	var err error

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err = utils.ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}

	backupID := flag.GetString(ctx, "backup-id")
	if backupID == "" {
		return fmt.Errorf("--backup-id flag is required")
	}

	fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)

	if cluster.Version == utils.V1 {
		return cmdv1.RunRestore(ctx, clusterID, backupID)
	}

	return cmdv2.RunRestore(ctx, clusterID, backupID)
}
