package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	uiexClient := uiexutil.ClientFromContext(ctx)

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
