package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newPlans() (cmd *cobra.Command) {
	const (
		long = `List available Redis plans`

		short = long
		usage = "plans"
	)

	cmd = command.New(usage, short, long, runPlans, command.RequireSession)

	flag.Add(cmd)

	return cmd
}

func runPlans(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	_ = `# @genqlient
  query ListAddOnPlans {
		addOnPlans {
			nodes {
				id
				displayName
			  maxDataSize
				pricePerMonth
			}
		}
  }
	`

	result, err := gql.ListAddOnPlans(ctx, client)

	var rows [][]string

	fmt.Fprintf(out, "\nRedis clusters run on Fly.io, fully managed by Upstash.com. \nOther limits, besides memory, apply to most plans. Learn more at https://fly.io/docs/reference/redis\n\n")

	for _, plan := range result.AddOnPlans.Nodes {

		row := []string{
			plan.DisplayName,
			plan.MaxDataSize,
		}

		var price string

		row = append(row, price)
		rows = append(rows, row)
	}

	_ = render.Table(out, "", rows, "Name", "Max Data Size")

	return
}
