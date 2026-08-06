package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	result, err := gql.ListAddOnPlans(ctx, client, gql.AddOnTypeUpstashRedis)
	if err != nil {
		return err
	}

	var rows [][]string

	fmt.Fprintf(out, "\nRedis databases run on Fly.io, fully managed by Upstash.com.\nOther limits, besides memory, apply to most plans. Learn more at https://fly.io/docs/reference/redis\n\n")

	for _, plan := range result.AddOnPlans.Nodes {
		// Filter out legacy plans - only show plans available for new databases
		if isLegacyPlan(plan.DisplayName) {
			continue
		}

		// Format price
		var price string
		if plan.PricePerMonth == 0 {
			price = "Free"
		} else {
			price = fmt.Sprintf("$%d/mo", plan.PricePerMonth/100)
		}

		row := []string{
			plan.DisplayName,
			plan.MaxDataSize,
			price,
			plan.Description,
		}

		rows = append(rows, row)
	}

	_ = render.Table(out, "", rows, "Name", "Max Data Size", "Price", "Description")

	return
}
