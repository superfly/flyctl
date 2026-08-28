package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
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

	request := flaps.RestoreManagedPostgresClusterRequest{
		BackupID: backupID,
		Name:     name,
		PITRTime: pitrTime,
	}

	response, err := flapsutil.ClientFromContext(ctx).RestoreManagedPostgresCluster(ctx, clusterID, request)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		publicErr := err
		var legacyResponse mpgv2.RestoreClusterBackupResponse
		legacyResponse, err = mpgClient.RestoreClusterBackup(ctx, clusterID, mpgv2.RestoreClusterBackupInput{
			BackupId: backupID,
			Name:     name,
			PitrTime: pitrTime,
		})
		if err != nil {
			return fmt.Errorf("failed to restore cluster: %w", publicErr)
		}

		response = flaps.ManagedPostgresCluster{
			ID:   legacyResponse.Data.Id,
			Name: legacyResponse.Data.Name,
		}
	}
	if err != nil {
		return fmt.Errorf("failed to restore cluster: %w", err)
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.ID)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Name)

	return nil
}
