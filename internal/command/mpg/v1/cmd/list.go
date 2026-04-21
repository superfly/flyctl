package cmdv1

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func NewList() *cobra.Command {
	const (
		long = `List MPG clusters owned by the specified organization.
If no organization is specified, the user's personal organization is used.`
		short = "List MPG clusters."
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
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

	mpgClient := mpgv1.ClientFromContext(ctx)
	genqClient := flyutil.ClientFromContext(ctx).GenqClient()

	// For ui-ex request we need the real org slug
	var fullOrg *gql.GetOrganizationResponse
	if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
		err = fmt.Errorf("failed fetching org: %w", err)

		return err
	}

	deleted := flag.GetBool(ctx, "deleted")
	clusters, err := mpgClient.ListManagedClusters(ctx, fullOrg.Organization.RawSlug, deleted)
	if err != nil {
		return fmt.Errorf("failed to list managed clusters for organization %s: %w", org.Slug, err)
	}

	if len(clusters.Data) == 0 {
		if deleted {
			fmt.Fprintf(out, "No deleted managed postgres clusters found in organization %s\n", org.Slug)
		} else {
			fmt.Fprintf(out, "No managed postgres clusters found in organization %s\n", org.Slug)
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

// formatAttachedApps formats the list of attached apps for display
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
