package builds

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() (cmd *cobra.Command) {
	const (
		long = `The builds list will list builds.
`
		short = "List builds"
	)

	cmd = command.New("list", short, long, runList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runList(ctx context.Context) (err error) {
	app := app.NameFromContext(ctx)

	client := client.FromContext(ctx).API()

	var builds []api.SourceBuild
	if builds, err = client.ListBuilds(ctx, app); err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg := config.FromContext(ctx); cfg.JSONOutput {
		_ = render.JSON(out, builds)

		return
	}

	rows := make([][]string, 0, len(builds))
	for _, build := range builds {
		rows = append(rows, []string{
			build.ID,
			build.Status,
		})
	}

	_ = render.Table(out, "", rows, "ID", "Status")

	return
}
