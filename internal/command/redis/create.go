package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new Redis instance`

		short = long
		usage = "create"
	)

	cmd = command.New(usage, short, long, runCreate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	primaryRegion, err := prompt.Region(ctx, "Choose a primary region (can't be changed later)")
	if err != nil {
		return err
	}

	readRegions, err := prompt.MultiRegion(ctx, "Optionally, choose one or more replica regions (can be changed later):", []string{}, primaryRegion.Code)
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

	url, err := ProvisionRedis(ctx, org, result.AddOnPlans.Nodes[index].Id, primaryRegion, readRegions)
	if err != nil {
		return
	}

	fmt.Fprintf(out, "Connect to your Upstash Redis instance at: %s\n", url)
	fmt.Fprintf(out, "This redis instance is visible to all applications in the %s organization.\n", org.Slug)

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization, planId string, primaryRegion *api.Region, readRegions *[]api.Region) (publicUrl string, err error) {
	client := client.FromContext(ctx).API().GenqClient

	_ = `# @genqlient
  mutation CreateAddOn($organizationId: ID!, $primaryRegion: String!, $planId: ID!, $readRegions: [String!]) {
		createAddOn(input: {organizationId: $organizationId, type: redis, planId: $planId, primaryRegion: $primaryRegion, readRegions: $readRegions}) {
			addOn {
				id
				publicUrl
			}
		}
  }
	`

	var readRegionCodes []string

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	response, err := gql.CreateAddOn(ctx, client, org.ID, primaryRegion.Code, planId, readRegionCodes)
	if err != nil {
		return
	}

	return response.CreateAddOn.AddOn.PublicUrl, nil
}
