package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long = `The APPS LIST command will show the applications currently
registered and available to this user. The list will include applications
from all the organizations the user is a member of. Each application will
be shown with its name, owner and when it was last deployed.
`
		short = "List applications"
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
	)

	flag.Add(cmd, flag.JSONOutput())
	flag.Add(cmd, flag.Org())

	cmd.Aliases = []string{"ls"}
	return cmd
}

func runList(ctx context.Context) (err error) {
	client := client.FromContext(ctx)
	cfg := config.FromContext(ctx)
	org, err := getOrg(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	var apps []api.App
	if org != nil {
		apps, err = client.API().GetAppsForOrganization(ctx, org.ID)
	} else {
		apps, err = client.API().GetApps(ctx, nil)
	}

	if err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		_ = render.JSON(out, apps)

		return
	}

	var verbose = flag.GetBool(ctx, "verbose")

	rows := make([][]string, 0, len(apps))
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
			app.PlatformVersion,
			latestDeploy,
		})
	}

	_ = render.Table(out, "", rows, "Name", "Owner", "Status", "Platform", "Latest Deploy")

	return
}

func getOrg(ctx context.Context) (*api.Organization, error) {
	client := client.FromContext(ctx).API()

	orgName := flag.GetOrg(ctx)

	if orgName == "" {
		return nil, nil
	}

	return client.GetOrganizationBySlug(ctx, orgName)
}
