package redis

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() (cmd *cobra.Command) {
	const (
		long  = `List Redis clusters for an organization`
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
		client = client.FromContext(ctx).API().GenqClient
	)

	_ = `# @genqlient
		query ListAddOns($addOnType: AddOnType) {
			addOns(type: $addOnType) {
				nodes {
					id
					name
					addOnPlan {
						displayName
					}
					primaryRegion
					readRegions
					organization {
						id
						slug
					}
				}
			}
		}
	`
	response, err := gql.ListAddOns(ctx, client, "redis")

	var rows [][]string

	for _, addon := range response.AddOns.Nodes {
		rows = append(rows, []string{
			addon.Id,
			addon.Name,
			addon.Organization.Slug,
			addon.AddOnPlan.DisplayName,
			addon.PrimaryRegion,
			strings.Join(addon.ReadRegions, ","),
		})
	}

	_ = render.Table(out, "", rows, "Id", "Name", "Org", "Plan", "Primary Region", "Read Regions")

	return
}
