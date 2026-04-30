package cmdv2

import (
	"context"
	"fmt"

	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// Copied from v1 RunRestore and changed client to mpgv2
func RunRestore(ctx context.Context, clusterID string, backupID string) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)

	input := mpgv2.RestoreManagedClusterBackupInput{
		BackupId: backupID,
	}

	response, err := mpgClient.RestoreManagedClusterBackup(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.Data.Id)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Data.Name)

	return nil
}
