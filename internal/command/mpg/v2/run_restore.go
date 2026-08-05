package cmdv2

import (
	"context"
	"fmt"

	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string, backupID string, name string, pitrTime string) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	if backupID != "" {
		fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)
	} else {
		fmt.Fprintf(out, "Restoring cluster %s to point in time %s...\n", clusterID, pitrTime)
	}

	input := mpgv2.RestoreClusterBackupInput{
		BackupId: backupID,
		Name:     name,
		PitrTime: pitrTime,
	}

	response, err := mpgClient.RestoreClusterBackup(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to restore cluster: %w", err)
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.Data.Id)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Data.Name)

	return nil
}
