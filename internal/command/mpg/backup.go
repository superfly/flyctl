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

func newBackup() *cobra.Command {
	const (
		short = "Manage Managed Postgres backups"
		long  = `List and create backups of a Managed Postgres cluster. To restore a
backup into a new cluster, use ` + "`fly mpg restore`" + `.`
	)

	cmd := command.New("backup", short, long, nil)
	cmd.Aliases = []string{"backups"}

	cmd.AddCommand(
		newBackupList(),
		newBackupCreate(),
	)

	return cmd
}

func newBackupList() *cobra.Command {
	const (
		long = `List backups of a Managed Postgres cluster with each backup's ID, start
time, status and type. By default only backups started in the last 24
hours are shown; pass --all to see every backup.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.

Restore a backup into a new cluster with ` + "`fly mpg restore --backup-id`" + `.`
		short = "List backups of a Managed Postgres cluster"
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runBackupList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "all",
			Description: "Show all backups (default: last 24 hours)",
			Default:     false,
		},
	)

	return cmd
}

func runBackupList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunBackupList(ctx, cluster.Id)
	}

	return cmdv2.RunBackupList(ctx, cluster.Id)
}

func newBackupCreate() *cobra.Command {
	const (
		long = `Queue a backup of a Managed Postgres cluster. The backup is full unless
--type incr asks for an incremental one. The command returns once the
backup is queued, not when it finishes; check on it with
` + "`fly mpg backup list`" + `.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.`
		short = "Create a backup of a Managed Postgres cluster"
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runBackupCreate,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "type",
			Description: "Backup type: full or incr",
			Default:     "full",
		},
	)

	return cmd
}

func runBackupCreate(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)

	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunBackupCreate(ctx, cluster.Id)
	}

	return cmdv2.RunBackupCreate(ctx, cluster.Id)
}
