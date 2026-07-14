package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long = `List the applications currently
available to this user. The list includes applications
from all the organizations the user is a member of. The list shows
the name, owner (org), status, and date/time of latest deploy for each app.
Apps on a non-default network also show the network name.
`
		short = "List applications."
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
	)

	flag.Add(cmd, flag.JSONOutput())
	flag.Add(cmd, flag.Org())
	flag.Add(cmd, flag.Bool{
		Name:        "quiet",
		Shorthand:   "q",
		Description: "Only list app names",
	})

	cmd.Aliases = []string{"ls"}

	return cmd
}

func runList(ctx context.Context) (err error) {
	silence := flag.GetBool(ctx, "quiet")
	cfg := config.FromContext(ctx)
	org, err := getOrg(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	var orgID *string
	if org != nil {
		orgID = &org.ID
	}

	apps, err := getApps(ctx, orgID)
	if err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		_ = render.JSON(out, apps)

		return
	}

	verbose := flag.GetBool(ctx, "verbose")

	rows := make([][]string, 0, len(apps))
	if silence {
		for _, app := range apps {
			rows = append(rows, []string{app.Name})
		}
		_ = render.Table(out, "", rows)

		return
	}
	showNetwork := false
	for _, app := range apps {
		if app.Network != "" {
			showNetwork = true

			break
		}
	}

	for _, app := range apps {
		latestDeploy := ""
		if app.Deployed && app.CurrentRelease != nil {
			latestDeploy = format.RelativeTime(app.CurrentRelease.CreatedAt)
		}

		if !verbose && strings.HasPrefix(app.Name, "flyctl-interactive-shells-") {
			app.Name = "(interactive shells app)"
		}

		row := []string{
			app.Name,
			app.Organization.Slug,
			app.Status,
		}
		if showNetwork {
			row = append(row, app.Network)
		}
		rows = append(rows, append(row, latestDeploy))
	}

	headers := []string{"Name", "Owner", "Status"}
	if showNetwork {
		headers = append(headers, "Network")
	}
	headers = append(headers, "Latest Deploy")

	_ = render.Table(out, "", rows, headers...)

	return
}

// getApps mirrors fly-go's GetApps/GetAppsForOrganization but also requests
// the network field, which those queries omit.
func getApps(ctx context.Context, orgID *string) ([]fly.App, error) {
	client := flyutil.ClientFromContext(ctx)

	query := `
		query($org: ID, $after: String) {
			apps(type: "container", first: 200, after: $after, organizationId: $org) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					name
					deployed
					hostname
					platformVersion
					network
					organization {
						slug
						name
					}
					currentRelease {
						createdAt
						status
					}
					status
				}
			}
		}
	`

	var apps []fly.App
	var after *string

	for {
		req := client.NewRequest(query)
		if orgID != nil {
			req.Var("org", *orgID)
		}
		if after != nil {
			req.Var("after", *after)
		}

		data, err := client.RunWithContext(ctx, req)
		if err != nil {
			return nil, err
		}

		apps = append(apps, data.Apps.Nodes...)

		if !data.Apps.PageInfo.HasNextPage {
			break
		}
		after = &data.Apps.PageInfo.EndCursor
	}

	return apps, nil
}

func getOrg(ctx context.Context) (*fly.Organization, error) {
	client := flyutil.ClientFromContext(ctx)

	orgName := flag.GetOrg(ctx)

	if orgName == "" {
		return nil, nil
	}

	return client.GetOrganizationBySlug(ctx, orgName)
}
