package enveloop

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your Enveloop projects`
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
	client := flyutil.ClientFromContext(ctx).GenqClient()
	response, err := gql.ListAddOns(ctx, client, "enveloop")
	if err != nil {
		return err
	}

	var rows [][]string
	for _, extension := range response.AddOns.Nodes {
		rows = append(rows, []string{
			extension.Name,
			extension.Organization.Slug,
			extension.PrimaryRegion,
		})
	}

	out := iostreams.FromContext(ctx).Out
	_ = render.Table(out, "", rows, "Name", "Org", "Region")

	return nil
}
