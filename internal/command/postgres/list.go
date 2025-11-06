package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List postgres clusters"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runList)

	flag.Add(cmd, flag.JSONOutput())
	cmd.Aliases = []string{"ls"}
	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		flapsClient = flapsutil.ClientFromContext(ctx)
		io          = iostreams.FromContext(ctx)
		cfg         = config.FromContext(ctx)
	)

	var apps []flaps.App

	uiexClient := uiexutil.ClientFromContext(ctx)
	orgs, err := uiexClient.ListOrganizations(ctx, false)
	if err != nil {
		return fmt.Errorf("error listing organizations: %w", err)
	}
	for _, org := range orgs {
		// todo(mapi): this is not ideal
		apps2, err := flapsClient.ListApps(ctx, org.RawSlug)
		if err != nil {
			return fmt.Errorf("error listing apps: %w", err)
		}
		for _, app := range apps2 {
			if app.AppRole == "postgres_cluster" {
				apps = append(apps, app)
			}
		}
	}

	if len(apps) == 0 {
		fmt.Fprintln(io.Out, "No postgres clusters found")
		return
	}

	// if --json
	if cfg.JSONOutput {
		return render.JSON(io.Out, apps)
	}

	rows := make([][]string, 0, len(apps))
	for _, app := range apps {
		latestDeploy := ""
		// todo(mapi)
		// if app.Deployed() && app.CurrentRelease != nil {
		// 	latestDeploy = format.RelativeTime(app.CurrentRelease.CreatedAt)
		// }

		rows = append(rows, []string{
			app.Name,
			app.Organization.Slug,
			app.Status,
			latestDeploy,
		})
	}

	_ = render.Table(io.Out, "", rows, "Name", "Owner", "Status", "Latest Deploy")

	return
}
