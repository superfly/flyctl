package builds

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() *cobra.Command {
	const (
		long  = `The builds list will list builds.`
		short = "List builds"
	)

	cmd := command.New("list", short, long, runList, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        "app",
			Description: "The app name to list builds on",
		},
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	cfg := config.FromContext(ctx)
	client := client.FromContext(ctx)

	var builds []api.SourceBuild
	if builds, err = client.API().ListBuilds(ctx, ""); err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
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
