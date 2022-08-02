package redis

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() (cmd *cobra.Command) {
	const (
		long  = `List Redis instances for an organization`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API()
	)

	services, err := client.GetThirdPartyServices(ctx, "upstash_redis")

	var rows [][]string

	for _, service := range services {
		rows = append(rows, []string{
			service.ID,
			service.Name,
			service.Organization.Slug,
			service.PrimaryRegion,
		})
	}

	_ = render.Table(out, "", rows, "Id", "Name", "Org", "Region")

	return
}
