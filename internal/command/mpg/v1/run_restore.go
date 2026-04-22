package cmdv1

import (
	"context"
	"fmt"

	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string, backupID string) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv1.ClientFromContext(ctx)

	input := mpgv1.RestoreManagedClusterBackupInput{
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
