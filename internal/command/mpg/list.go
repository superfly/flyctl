package mpg

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		long = `List the Managed Postgres clusters in an organization, with each
cluster's ID, region, status, plan and attached apps.

Pass the organization with --org. Without it, you pick one from a list in
a terminal; without a terminal, --org is required. Use --deleted to list
deleted clusters instead of active ones.`
		short = "List Managed Postgres clusters in an organization"
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())
	flag.Add(cmd, flag.Org())
	flag.Add(cmd, flag.Bool{
		Name:        "deleted",
		Description: "Show deleted clusters instead of active clusters",
		Default:     false,
	})

	return cmd
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}
	orgSlug := org.Slug

	deleted := flag.GetBool(ctx, "deleted")
	// For ui-ex request we need the real org slug
	clusters, err := listManagedClusters(ctx, org.RawSlug, deleted)
	if err != nil {
		return fmt.Errorf("failed to list postgres clusters for organization %s: %w", orgSlug, err)
	}

	if len(clusters) == 0 {
		if deleted {
			fmt.Fprintf(out, "No deleted managed postgres clusters found in organization %s\n", orgSlug)
		} else {
			fmt.Fprintf(out, "No managed postgres clusters found in organization %s\n", orgSlug)
		}

		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, clusters)
	}

	rows := make([][]string, 0, len(clusters))
	for _, cluster := range clusters {
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
func FormatAttachedApps(apps []mpg.AttachedApp) string {
	if len(apps) == 0 {
		return "<no attached apps>"
	}

	names := make([]string, len(apps))
	for i, app := range apps {
		names[i] = app.Name
	}

	return strings.Join(names, ", ")
}
