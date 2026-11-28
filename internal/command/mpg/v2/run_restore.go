package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mpgutil"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string, backupID string, name string, pitrTime string) error {
	io := iostreams.FromContext(ctx)
	jsonOutput := config.FromContext(ctx).JSONOutput
	out := io.Out
	if jsonOutput {
		out = io.ErrOut
	}
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
			ID:           legacyResponse.Data.Id,
			Name:         legacyResponse.Data.Name,
			Region:       legacyResponse.Data.Region,
			Plan:         legacyResponse.Data.Plan,
			Status:       legacyResponse.Data.Status,
			Organization: flaps.ManagedPostgresOrganization{Name: legacyResponse.Data.Organization.Name, Slug: legacyResponse.Data.Organization.Slug},
		}
	}
	if err != nil {
		return fmt.Errorf("failed to restore cluster: %w", err)
	}

	if jsonOutput {
		return mpgutil.RenderCreatedJSON(io.Out, fmt.Sprintf("cluster %s", response.ID), mpgutil.ClusterResult{
			ID: response.ID, Name: response.Name,
			Organization: mpgutil.ClusterOrganization{Name: response.Organization.Name, Slug: response.Organization.Slug},
			Region:       response.Region, Plan: response.Plan, Status: response.Status,
		})
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.ID)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Name)

	return nil
}
