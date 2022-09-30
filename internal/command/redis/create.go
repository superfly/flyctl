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
		long = `Create an Upstash Redis database`

		short = long
		usage = "create"
	)

	cmd = command.New(usage, short, long, runCreate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Redis database",
		},
		flag.Bool{
			Name:        "no-replicas",
			Description: "No replica regions",
		},
		flag.Bool{
			Name:        "enable-eviction",
			Description: "Evict objects when memory is full",
		},
		flag.Bool{
			Name:        "disable-eviction",
			Description: "Disallow writes when the max data size limit has been reached",
		},
		flag.String{
			Name:        "plan",
			Description: "Upstash Redis plan",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		client   = client.FromContext(ctx).API().GenqClient
		colorize = io.ColorScheme()
	)

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	var name = flag.GetString(ctx, "name")

	if name == "" {
		prompt.String(ctx, &name, "Choose a Redis database name (leave blank to generate one):", "", false)

		if err != nil {
			return err
		}
	}

	_ = `# @genqlient
	query GetAddOnProvider($name: String!) {
		addOnProvider(name: $name) {
			id
			name
			excludedRegions {
				code
			}
		}
	}
	`

	response, err := gql.GetAddOnProvider(ctx, client, "upstash_redis")

	if err != nil {
		return err
	}

	var excludedRegions []string

	for _, region := range response.AddOnProvider.ExcludedRegions {
		excludedRegions = append(excludedRegions, region.Code)
	}

	primaryRegion, err := prompt.Region(ctx, prompt.RegionParams{
		Message:             "Choose a primary region (can't be changed later)",
		ExcludedRegionCodes: excludedRegions,
	})

	if err != nil {
		return err
	}

	readRegions := &[]api.Region{}

	excludedRegions = append(excludedRegions, primaryRegion.Code)

	if !flag.GetBool(ctx, "no-replicas") {
		readRegions, err = prompt.MultiRegion(ctx, "Optionally, choose one or more replica regions (can be changed later):", []string{}, excludedRegions)

		if err != nil {
			return
		}
	}

	var eviction bool

	if flag.GetBool(ctx, "enable-eviction") {
		eviction = true
	} else if !flag.GetBool(ctx, "disable-eviction") {
		fmt.Fprintf(io.Out, "\nUpstash Redis can evict objects when memory is full. This is useful when caching in Redis. This setting can be changed later.\nLearn more at https://fly.io/docs/reference/redis/#memory-limits-and-object-eviction-policies\n")

		eviction, err = prompt.Confirm(ctx, "Would you like to enable eviction?")
		if err != nil {
			return
		}
	}

	var planIndex int

	result, err := gql.ListAddOnPlans(ctx, client)
	if err != nil {
		return
	}

	planFlag := flag.GetString(ctx, "plan")

	if planFlag != "" {
		planIndex = -1
		for index, plan := range result.AddOnPlans.Nodes {
			if plan.DisplayName == planFlag {
				planIndex = index
				break
			}
		}

		if planIndex == -1 {
			return fmt.Errorf("invalid plan name: %s", planFlag)
		}
	} else {
		var planOptions []string

		for _, plan := range result.AddOnPlans.Nodes {
			planOptions = append(planOptions, fmt.Sprintf("%s: %s Max Data Size", plan.DisplayName, plan.MaxDataSize))
		}

		err = prompt.Select(ctx, &planIndex, "Select an Upstash Redis plan", "", planOptions...)

		if err != nil {
			return fmt.Errorf("failed to select a plan: %w", err)
		}
	}

	s := spinner.Run(io, "Launching...")

	addOn, err := ProvisionRedis(ctx, org, name, result.AddOnPlans.Nodes[planIndex].Id, primaryRegion, readRegions, eviction)

	s.Stop()
	if err != nil {
		return
	}

	fmt.Fprintf(io.Out, "\nYour Upstash Redis database %s is ready.\n", colorize.Green(addOn.Name))
	fmt.Fprintf(io.Out, "Apps in the %s org can connect to at %s\n", colorize.Green(org.Slug), colorize.Green(addOn.PublicUrl))
	fmt.Fprintf(io.Out, "If you have redis-cli installed, use %s to connect to your database.\n", colorize.Green("fly redis connect"))

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization, name string, planId string, primaryRegion *api.Region, readRegions *[]api.Region, eviction bool) (addOn gql.CreateAddOnCreateAddOnCreateAddOnPayloadAddOn, err error) {
	client := client.FromContext(ctx).API().GenqClient

	_ = `# @genqlient
  mutation CreateAddOn($organizationId: ID!, $primaryRegion: String!, $name: String, $planId: ID!, $readRegions: [String!], $options: JSON!) {
		createAddOn(input: {organizationId: $organizationId, type: redis, name: $name, planId: $planId, primaryRegion: $primaryRegion,
				readRegions: $readRegions, options: $options}) {
			addOn {
				name
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

	response, err := gql.CreateAddOn(ctx, client, org.ID, primaryRegion.Code, name, planId, readRegionCodes, options)
	if err != nil {
		return
	}

	return response.CreateAddOn.AddOn, nil
}
