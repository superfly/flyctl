package tigris

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/lib/command"
	extensions_core "github.com/superfly/flyctl/lib/command/extensions/core"
	"github.com/superfly/flyctl/lib/flag"
	"github.com/superfly/flyctl/lib/flyutil"
	"github.com/superfly/flyctl/lib/render"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your Tigris object storage buckets`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
		extensions_core.SharedFlags,
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	response, err := gql.ListAddOns(ctx, client, "tigris")

	var rows [][]string

	for _, extension := range response.AddOns.Nodes {
		rows = append(rows, []string{
			extension.Name,
			extension.Organization.Slug,
		})
	}

	_ = render.Table(out, "", rows, "Name", "Org")

	return
}
