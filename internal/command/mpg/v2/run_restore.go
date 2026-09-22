package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string, backupID string, name string, pitrTime string) error {
	out := iostreams.FromContext(ctx).Out

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
	if err != nil {
		return fmt.Errorf("failed to restore cluster: %w", err)
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.ID)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Name)

	return nil
}
