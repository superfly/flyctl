package cmdv1

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func RunList(ctx context.Context, orgSlug string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	mpgClient := mpgv1.ClientFromContext(ctx)
	genqClient := flyutil.ClientFromContext(ctx).GenqClient()

	// For ui-ex request we need the real org slug
	var fullOrg *gql.GetOrganizationResponse
	var err error
	if fullOrg, err = gql.GetOrganization(ctx, genqClient, orgSlug); err != nil {
		return fmt.Errorf("failed fetching org: %w", err)
	}

	deleted := flag.GetBool(ctx, "deleted")
	clusters, err := mpgClient.ListManagedClusters(ctx, fullOrg.Organization.RawSlug, deleted)
	if err != nil {
		return fmt.Errorf("failed to list managed clusters for organization %s: %w", orgSlug, err)
	}

	if len(clusters.Data) == 0 {
		if deleted {
			fmt.Fprintf(out, "No deleted managed postgres clusters found in organization %s\n", orgSlug)
		} else {
			fmt.Fprintf(out, "No managed postgres clusters found in organization %s\n", orgSlug)
		}

		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, clusters.Data)
	}

	rows := make([][]string, 0, len(clusters.Data))
	for _, cluster := range clusters.Data {
		rows = append(rows, []string{
			cluster.Id,
			cluster.Name,
			cluster.Organization.Slug,
			cluster.Region,
			cluster.Status,
			cluster.Plan,
			FormatAttachedApps(cluster.AttachedApps),
		})
	}

	return render.Table(out, "", rows, "ID", "Name", "Org", "Region", "Status", "Plan", "Attached Apps")
}

// FormatAttachedApps formats the list of attached apps for display
func FormatAttachedApps(apps []mpgv1.AttachedApp) string {
	if len(apps) == 0 {
		return "<no attached apps>"
	}

	names := make([]string, len(apps))
	for i, app := range apps {
		names[i] = app.Name
	}

	return strings.Join(names, ", ")
}
