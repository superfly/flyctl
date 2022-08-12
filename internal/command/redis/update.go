package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newUpdate() (cmd *cobra.Command) {
	const (
		long = `Update an Upstash Redis instance`

		short = long
		usage = "update <id>"
	)

	cmd = command.New(usage, short, long, runUpdate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	id := flag.FirstArg(ctx)

	_ = `# @genqlient
	query GetAddOn($id: ID!) {
		addOn(id: $id) {
			id
			name
			publicUrl
			primaryRegion
			readRegions
			addOnPlan {
				id
				name
				displayName
			}
		}
	}
`
	response, err := gql.GetAddOn(ctx, client, id)
	if err != nil {
		return
	}

	addOn := response.AddOn

	readRegions, err := prompt.MultiRegion(ctx, "Choose replica regions, or unselect to remove replica regions:", addOn.ReadRegions, addOn.PrimaryRegion)
	if err != nil {
		return
	}

	var index int
	var promptOptions []string

	result, err := gql.ListAddOnPlans(ctx, client)
	if err != nil {
		return
	}

	for _, plan := range result.AddOnPlans.Nodes {
		promptOptions = append(promptOptions, fmt.Sprintf("%s: %s Max Data Size, $%d/month/region", plan.DisplayName, plan.MaxDataSize, plan.PricePerMonth))
	}

	err = prompt.Select(ctx, &index, "Select an Upstash Redis plan", "", promptOptions...)

	if err != nil {
		return fmt.Errorf("failed to select a plan: %w", err)
	}

	_ = `# @genqlient
  mutation UpdateAddOn($addOnId: ID!, $planId: ID!, $readRegions: [String!]) {
		updateAddOn(input: {addOnId: $addOnId, planId: $planId, readRegions: $readRegions}) {
			addOn {
				id
			}
		}
  }
	`

	var readRegionCodes []string

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, result.AddOnPlans.Nodes[index].Id, readRegionCodes)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Upstash Redis instance %s was updated.\n", addOn.Id)

	return
}
