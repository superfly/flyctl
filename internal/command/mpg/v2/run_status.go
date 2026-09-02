package cmdv2

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// RunStatus shows cluster status.
//
// Human output prefers the public Machines API; the legacy MPGv2 client is
// used only as a fallback when the public API returns a classified 404.
// Other public API errors are propagated without falling back.
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
	useLegacy := errors.Is(err, flaps.ErrFlapsNotFound)
	if err != nil && !useLegacy {
		return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, err)
	}

	var (
		id, name, org, region, statusVal, directIP string
		diskGB, replicas                           int
	)

	if useLegacy {
		legacyResp, legacyErr := mpgv2.ClientFromContext(ctx).GetClusterById(ctx, clusterID)
		if legacyErr != nil {
			return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, legacyErr)
		}
		id = legacyResp.Data.Id
		name = legacyResp.Data.Name
		org = legacyResp.Data.Organization.Slug
		region = legacyResp.Data.Region
		statusVal = legacyResp.Data.Status
		diskGB = legacyResp.Data.Disk
		replicas = legacyResp.Data.Replicas
		directIP = legacyResp.Data.IpAssignments.Direct
	} else {
		id = cluster.ID
		name = cluster.Name
		org = cluster.Organization.Slug
		region = cluster.Region
		statusVal = cluster.Status
		diskGB = cluster.DiskSizeGB
		replicas = cluster.Replicas
		// Render the public endpoint host only to preserve the legacy Direct
		// column's bare-address and empty-host behavior.
		directIP = cluster.Endpoints.Primary.Direct.Host
	}

	rows := [][]string{{
		id,
		name,
		org,
		region,
		statusVal,
		strconv.Itoa(diskGB),
		strconv.Itoa(replicas),
		directIP,
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
