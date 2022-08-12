package redis

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show status of a Redis service"
		long  = short + "\n"

		usage = "status <id>"
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
		id     = flag.FirstArg(ctx)
		client = client.FromContext(ctx).API().GenqClient
	)

	response, err := gql.GetAddOn(ctx, client, id)

	if err != nil {
		return err
	}

	addOn := response.AddOn

	obj := [][]string{
		{
			addOn.Id,
			addOn.Name,
			addOn.AddOnPlan.DisplayName,
			addOn.PrimaryRegion,
			strings.Join(addOn.ReadRegions, ","),
			addOn.PublicUrl,
		},
	}

	var cols []string = []string{"ID", "Name", "Plan", "Primary Region", "Read Regions", "Public URL"}

	if err = render.VerticalTable(io.Out, "Redis", obj, cols...); err != nil {
		return
	}

	return
}
