package fly_mysql

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func status() *cobra.Command {
	const (
		short = "Show details about a MySQL database"
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
		extensions_core.SharedFlags,
	)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	extension, app, err := extensions_core.Discover(ctx, gql.AddOnTypeFlyMysql)
	if err != nil {
		return err
	}

	options, _ := extension.Options.(map[string]interface{})

	obj := [][]string{
		{
			extension.Name,
			extension.PrimaryRegion,
			extension.Status,
		},
	}

	cols := []string{"Name", "Primary Region", "Status"}

	if app != nil {
		obj[0] = append(obj[0], app.Name)
		cols = append(cols, "App")
	}

	if options != nil {

		for _, v := range []string{"size", "cpu", "memory", "disk"} {
			var unit string
			if options[v] != nil {
				if v == "size" {
					unit = "members"
				} else if v == "cpu" {
					unit = "cores"
				} else {
					unit = "GB"
				}
				obj[0] = append(obj[0], fmt.Sprintf("%d %s", int(options[v].(float64)), unit))
				cols = append(cols, v)
			}
		}

	}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}

	return
}
