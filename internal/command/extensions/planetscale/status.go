package planetscale

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func status() *cobra.Command {
	const (
		short = "Show details about a PlanetScale MySQL database"
		long  = short + "\n"

		usage = "status [name]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession, command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	var (
		io = iostreams.FromContext(ctx)
	)

	extension, app, err := extensions_core.Discover(ctx)

	if err != nil {
		return err
	}

	var appName string

	if app != nil {
		appName = app.Name
	}

	obj := [][]string{
		{
			extension.Name,
			extension.PrimaryRegion,
			extension.Status,
			appName,
		},
	}

	var cols []string = []string{"Name", "Primary Region", "Status", "App"}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}

	return
}
