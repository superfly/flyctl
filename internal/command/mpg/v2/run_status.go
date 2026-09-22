package cmdv2

import (
	"context"
	"fmt"
	"strconv"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// RunStatus shows cluster status.
//
// Human output uses the public Machines API.
//
// --json skips the public API because the legacy response envelope carries
// private credentials the public API does not expose; reusing the legacy
// client preserves the existing JSON shape byte-for-byte.
func RunStatus(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)

	if cfg.JSONOutput {
		return runStatusLegacy(ctx, clusterID)
	}

	return runStatusHuman(ctx, clusterID)
}

func runStatusHuman(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out

	cluster, err := flapsutil.ClientFromContext(ctx).GetManagedPostgresCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, err)
	}

	rows := [][]string{{
		cluster.ID,
		cluster.Name,
		cluster.Organization.Slug,
		cluster.Region,
		cluster.Status,
		strconv.Itoa(cluster.DiskSizeGB),
		strconv.Itoa(cluster.Replicas),
		// Render the public endpoint host only to preserve the legacy Direct
		// column's bare-address and empty-host behavior.
		cluster.Endpoints.Primary.Direct.Host,
	}}

	return render.VerticalTable(out, "Cluster Status", rows,
		"ID",
		"Name",
		"Organization",
		"Region",
		"Status",
		"Allocated Disk (GB)",
		"Replicas",
		"Direct IP",
	)
}

// runStatusLegacy renders the legacy MPGv2 response as JSON, preserving its
// historical shape (including the credentials envelope).
func runStatusLegacy(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out

	clusterDetails, err := mpgv2.ClientFromContext(ctx).GetClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, err)
	}

	return render.JSON(out, clusterDetails)
}
