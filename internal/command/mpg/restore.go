package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newRestore() *cobra.Command {
	const (
		long = `Restore a backup of a Managed Postgres cluster into a new, separate cluster.
The source cluster is not modified. The restored cluster is provisioned in the
same organization and billed as a separate cluster. Provisioning happens
asynchronously; the new cluster ID and generated name are printed when the
restore is initiated.

Find backup IDs with 'fly mpg backup list <CLUSTER_ID>', or
'fly mpg backup list <CLUSTER_ID> --all' for backups older than 24 hours.`
		short = "Restore MPG cluster from backup into a new cluster."
		usage = "restore <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runRestore,
		command.RequireSession,
		requireMacaroonToken,
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
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	backupID := flag.GetString(ctx, "backup-id")
	if backupID == "" {
		return fmt.Errorf("--backup-id flag is required")
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunRestore(ctx, cluster.Id, backupID)

	}

	return cmdv2.RunRestore(ctx, cluster.Id, backupID)
}
