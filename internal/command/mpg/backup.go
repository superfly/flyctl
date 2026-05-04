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
		short = "Backup commands"
		long  = short + "\n"
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
		long  = `List backups for a Managed Postgres cluster.`
		short = "List MPG cluster backups."
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runBackupList,
		command.RequireSession,
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
		long  = `Create a backup for a Managed Postgres cluster.`
		short = "Create MPG cluster backup."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runBackupCreate,
		command.RequireSession,
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

	return cmdv2.RunBackupCreate(ctx, clusterID)
}
