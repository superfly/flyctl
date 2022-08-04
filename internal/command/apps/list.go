package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
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

	return command.New("list", short, long, runList,
		command.RequireSession,
	)
}

func runList(ctx context.Context) (err error) {
	cfg := config.FromContext(ctx)
	client := client.FromContext(ctx)

	var apps []api.App
	if apps, err = client.API().GetApps(ctx, nil); err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		_ = render.JSON(out, apps)

		return
	}

	rows := make([][]string, 0, len(apps))
	for _, app := range apps {
		latestDeploy := ""
		if app.Deployed && app.CurrentRelease != nil {
			latestDeploy = format.RelativeTime(app.CurrentRelease.CreatedAt)
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
