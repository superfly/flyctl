package mpg

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newRestore() *cobra.Command {
	const (
		long = `Restore a Managed Postgres cluster from a backup or a point in time into a
new cluster, leaving the source cluster unchanged. The restored cluster is
provisioned asynchronously in the same organization and billed separately.

Find backup IDs with 'fly mpg backup list'.`
		short = "Restore MPG cluster from a backup or a point in time into a new cluster."
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
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the restored cluster (defaults to a generated name)",
		},
		flag.String{
			Name:        "pitr-time",
			Description: "Restore to a specific point in time (RFC3339, e.g. 2026-06-01T12:00:00Z). Requires the cluster's PITR recovery window to cover this time. Mutually exclusive with --backup-id.",
		},
	)

	return cmd
}

func runRestore(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	backupID := flag.GetString(ctx, "backup-id")
	pitrTime := flag.GetString(ctx, "pitr-time")
	name := flag.GetString(ctx, "name")
	if backupID == "" && pitrTime == "" {
		return fmt.Errorf("one of --backup-id or --pitr-time is required")
	}
	if backupID != "" && pitrTime != "" {
		return fmt.Errorf("--backup-id and --pitr-time are mutually exclusive")
	}
	if pitrTime != "" {
		if _, err := time.Parse(time.RFC3339, pitrTime); err != nil {
			return fmt.Errorf("--pitr-time must be an RFC3339 timestamp with an explicit offset (e.g. 2026-06-01T12:00:00Z): %w", err)
		}
	}

	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}
	if pitrTime != "" && cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("point-in-time restore is not supported for this cluster")
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunRestore(ctx, cluster.Id, backupID, name)

	}

	return cmdv2.RunRestore(ctx, cluster.Id, backupID, name, pitrTime)
}
