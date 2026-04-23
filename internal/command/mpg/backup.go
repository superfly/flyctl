package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
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
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunBackupList(ctx, clusterID)
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
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunBackupCreate(ctx, clusterID)
}
