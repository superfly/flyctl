package cmdv1

import (
	"context"
	"fmt"
	"strconv"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func RunStatus(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv1.ClientFromContext(ctx)

	// Fetch detailed cluster information by ID
	clusterDetails, err := mpgClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, err)
	}

	if cfg.JSONOutput {
		return render.JSON(out, clusterDetails)
	}

	rows := [][]string{{
		clusterDetails.Data.Id,
		clusterDetails.Data.Name,
		clusterDetails.Data.Organization.Slug,
		clusterDetails.Data.Region,
		clusterDetails.Data.Status,
		strconv.Itoa(clusterDetails.Data.Disk),
		strconv.Itoa(clusterDetails.Data.Replicas),
		clusterDetails.Data.IpAssignments.Direct,
	}}

	cols := []string{
		"ID",
		"Name",
		"Organization",
		"Region",
		"Status",
		"Allocated Disk (GB)",
		"Replicas",
		"Direct IP",
	}

	return render.VerticalTable(out, "Cluster Status", rows, cols...)
}
