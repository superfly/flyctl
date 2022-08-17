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
	"github.com/superfly/flyctl/internal/spinner"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new Redis database`

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
		io     = iostreams.FromContext(ctx)
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

	fmt.Fprintf(io.Out, "\nUpstash Redis can evict objects when memory is full. This is useful when caching in Redis. This setting can be changed later.\nLearn more at https://fly.io/docs/reference/redis/#memory-limits-and-object-eviction-policies\n")

	eviction, err := prompt.Confirm(ctx, "Would you like to enable eviction?")
	if err != nil {
		return
	}

	var planIndex int
	var planOptions []string

	result, err := gql.ListAddOnPlans(ctx, client)
	if err != nil {
		return
	}

	for _, plan := range result.AddOnPlans.Nodes {
		planOptions = append(planOptions, fmt.Sprintf("%s: %s Max Data Size", plan.DisplayName, plan.MaxDataSize))
	}

	err = prompt.Select(ctx, &planIndex, "Select an Upstash Redis plan", "", planOptions...)

	if err != nil {
		return fmt.Errorf("failed to select a plan: %w", err)
	}

	s := spinner.Run(io, "Launching...")

	url, err := ProvisionRedis(ctx, org, result.AddOnPlans.Nodes[planIndex].Id, primaryRegion, readRegions, eviction)

	s.Stop()
	if err != nil {
		return
	}

	fmt.Fprintf(io.Out, "\nConnect to your Upstash Redis database at: %s\n", url)
	fmt.Fprintf(io.Out, "Run \"fly redis connect\" command to connect to your database with redis-cli.\n")
	fmt.Fprintf(io.Out, "This redis database is visible to all applications in the %s organization.\n", org.Slug)

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization, planId string, primaryRegion *api.Region, readRegions *[]api.Region, eviction bool) (publicUrl string, err error) {
	client := client.FromContext(ctx).API().GenqClient

	_ = `# @genqlient
  mutation CreateAddOn($organizationId: ID!, $primaryRegion: String!, $planId: ID!, $readRegions: [String!], $options: JSON!) {
		createAddOn(input: {organizationId: $organizationId, type: redis, planId: $planId, primaryRegion: $primaryRegion,
				readRegions: $readRegions, options: $options}) {
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

	type AddOnOptions map[string]interface{}
	options := AddOnOptions{}

	if eviction {
		options["eviction"] = true
	}

	response, err := gql.CreateAddOn(ctx, client, org.ID, primaryRegion.Code, planId, readRegionCodes, options)
	if err != nil {
		return
	}

	return response.CreateAddOn.AddOn.PublicUrl, nil
}
