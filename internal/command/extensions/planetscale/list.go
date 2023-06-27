package planetscale

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your provisioned PlanetScale MySQL databases`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	response, err := gql.ListAddOns(ctx, client, "planetscale")

	var rows [][]string

	for _, addon := range response.AddOns.Nodes {
		rows = append(rows, []string{
			addon.Name,
			addon.Organization.Slug,
			addon.PrimaryRegion,
		})
	}

	_ = render.Table(out, "", rows, "Name", "Org", "Primary Region")

	return
}
