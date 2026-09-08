package apps

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/sort"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
	apps, err := getApps(ctx, flag.GetOrg(ctx))
	if err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		return render.JSON(out, apps)
	}

	if len(apps) == 0 {
		if !silence {
			fmt.Fprintln(out, "No apps found")
		}

		return nil
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

// getApps lists the apps the user can see, or only those in orgSlug when it
// is set, through Flaps. The latest deploy time comes from the ui-ex
// current-release timestamps, which cover every app in one request.
func getApps(ctx context.Context, orgSlug string) ([]fly.App, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Flaps reports raw org slugs; show the slug the user knows the org by,
	// which is "personal" for their personal org, as the GraphQL listing did.
	var orgs []uiex.Organization
	if orgSlug == "" {
		var err error
		if orgs, err = uiexClient.ListOrganizations(ctx, false); err != nil {
			return nil, err
		}
	} else {
		org, err := uiexClient.GetOrganization(ctx, orgSlug)
		if err != nil {
			return nil, err
		}
		orgs = []uiex.Organization{*org}
	}
	// The GraphQL listing grouped apps by org, personal org first, and
	// sorted by name within each org; keep that order.
	sort.OrganizationsByTypeAndName(orgs)
	slugByRawSlug := make(map[string]string, len(orgs))
	for _, org := range orgs {
		slugByRawSlug[org.RawSlug] = org.Slug
	}

	releaseTimes, err := uiexClient.GetAllAppsCurrentReleaseTimestamps(ctx)
	if err != nil {
		return nil, err
	}

	apps := []fly.App{}
	for _, org := range orgs {
		flapsApps, err := flapsClient.ListApps(ctx, flaps.ListAppsRequest{OrgSlug: org.Slug})
		if err != nil {
			return nil, err
		}
		slices.SortFunc(flapsApps, func(a, b flaps.App) int { return cmp.Compare(a.Name, b.Name) })

		for _, app := range flapsApps {
			// The GraphQL app ID is the app name.
			out := fly.App{
				ID:       app.Name,
				Name:     app.Name,
				Deployed: app.Deployed(),
				Status:   app.Status,
				Network:  flapsutil.NetworkName(&app),
				Organization: fly.Organization{
					Slug: cmp.Or(slugByRawSlug[app.Organization.Slug], app.Organization.Slug),
					Name: app.Organization.Name,
				},
			}
			if releaseTimes != nil {
				if createdAt, ok := (*releaseTimes)[app.Name]; ok && !createdAt.IsZero() {
					out.CurrentRelease = &fly.Release{CreatedAt: createdAt}
				}
			}
			apps = append(apps, out)
		}
	}

	return apps, nil
}
