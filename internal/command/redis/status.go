package redis

import (
	"context"
	"strconv"
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
		short = "Show status of a Redis database"
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

	var readRegions string = "None"

	if len(addOn.ReadRegions) > 0 {
		readRegions = strings.Join(addOn.ReadRegions, ",")
	}

	options, _ := addOn.Options.(map[string]interface{})

	obj := [][]string{
		{
			addOn.Id,
			addOn.Name,
			addOn.AddOnPlan.DisplayName,
			addOn.PrimaryRegion,
			readRegions,
			strconv.FormatBool(options["eviction"].(bool)),
			addOn.PublicUrl,
		},
	}

	var cols []string = []string{"ID", "Name", "Plan", "Primary Region", "Read Regions", "Eviction", "Private URL"}

	if err = render.VerticalTable(io.Out, "Redis", obj, cols...); err != nil {
		return
	}

	return
}
