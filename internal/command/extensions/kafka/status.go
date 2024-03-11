package kafka

import (
	"context"

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
		short = "Show details about an Upstash Kafka cluster"
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

	extension, app, err := extensions_core.Discover(ctx, gql.AddOnTypeUpstashKafka)
	if err != nil {
		return err
	}

	obj := [][]string{
		{
			extension.Name,
			extension.Status,
			extension.PrimaryRegion,
		},
	}

	var cols []string = []string{"Name", "Status", "Region"}

	if app != nil {
		obj[0] = append(obj[0], app.Name)
		cols = append(cols, "App")
	}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}
	return
}
