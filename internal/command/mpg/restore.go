package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
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
		command.RequireUiex,
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
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	uiexClient := uiexutil.ClientFromContext(ctx)

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	backupID := flag.GetString(ctx, "backup-id")
	if backupID == "" {
		return fmt.Errorf("--backup-id flag is required")
	}

	fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)

	input := uiex.RestoreManagedClusterBackupInput{
		BackupId: backupID,
	}

	response, err := uiexClient.RestoreManagedClusterBackup(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.Data.Id)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Data.Name)

	return nil
}
