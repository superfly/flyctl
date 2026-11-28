package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/mpgutil"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func RunRestore(ctx context.Context, clusterID string, backupID string, name string) error {
	io := iostreams.FromContext(ctx)
	jsonOutput := config.FromContext(ctx).JSONOutput
	out := io.Out
	if jsonOutput {
		out = io.ErrOut
	}
	mpgClient := mpgv1.ClientFromContext(ctx)

	fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)

	input := mpgv1.RestoreManagedClusterBackupInput{
		BackupId: backupID,
		Name:     name,
	}

	response, err := mpgClient.RestoreManagedClusterBackup(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to restore cluster: %w", err)
	}

	if jsonOutput {
		return mpgutil.RenderCreatedJSON(io.Out, fmt.Sprintf("cluster %s", response.Data.Id), mpgutil.ClusterResult{
			ID: response.Data.Id, Name: response.Data.Name, Organization: mpgutil.ClusterOrganization{Name: response.Data.Organization.Name, Slug: response.Data.Organization.Slug},
			Region: response.Data.Region, Plan: response.Data.Plan, Status: response.Data.Status,
		})
	}

	fmt.Fprintf(out, "Restore initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.Data.Id)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Data.Name)

	return nil
}
