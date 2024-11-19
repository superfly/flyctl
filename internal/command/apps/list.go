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
	client := flyutil.ClientFromContext(ctx)
	silence := flag.GetBool(ctx, "quiet")
	cfg := config.FromContext(ctx)
	org, err := getOrg(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	var apps []fly.App
	if org != nil {
		apps, err = client.GetAppsForOrganization(ctx, org.ID)
	} else {
		apps, err = client.GetApps(ctx, nil)
	}

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
	for _, app := range apps {
		latestDeploy := ""
		if app.Deployed && app.CurrentRelease != nil {
			latestDeploy = format.RelativeTime(app.CurrentRelease.CreatedAt)
		}

		if !verbose && strings.HasPrefix(app.Name, "flyctl-interactive-shells-") {
			app.Name = "(interactive shells app)"
		}

		rows = append(rows, []string{
			app.Name,
			app.Organization.Slug,
			app.Status,
			latestDeploy,
		})
	}

	_ = render.Table(out, "", rows, "Name", "Owner", "Status", "Latest Deploy")

	return
}

func getOrg(ctx context.Context) (*fly.Organization, error) {
	client := flyutil.ClientFromContext(ctx)

	orgName := flag.GetOrg(ctx)

	if orgName == "" {
		return nil, nil
	}

	return client.GetOrganizationBySlug(ctx, orgName)
}
