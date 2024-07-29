package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
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
		client = flyutil.ClientFromContext(ctx)
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
	)

	apps, err := client.GetApps(ctx, fly.StringPointer("postgres_cluster"))
	if err != nil {
		return fmt.Errorf("failed to list postgres clusters: %w", err)
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
		if app.Deployed && app.CurrentRelease != nil {
			latestDeploy = format.RelativeTime(app.CurrentRelease.CreatedAt)
		}

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
