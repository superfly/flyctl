package extensions

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatusPlanetscale() *cobra.Command {
	const (
		short = "Show status of a PlanetScale database"
		long  = short + "\n"

		usage = "status <name>"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		name   = flag.FirstArg(ctx)
		client = client.FromContext(ctx).API().GenqClient
	)

	response, err := gql.GetAddOn(ctx, client, name)
	if err != nil {
		return err
	}

	addOn := response.AddOn

	obj := [][]string{
		{
			addOn.Name,
			addOn.PrimaryRegion,
			addOn.Status,
		},
	}

	var cols []string = []string{"Name", "Primary Region", "Status"}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}

	return
}
