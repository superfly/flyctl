package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
)

func newUpdate() (cmd *cobra.Command) {
	const (
		long = `Update an Upstash Redis database`

		short = long
		usage = "update <name>"
	)

	cmd = command.New(usage, short, long, runUpdate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
		flag.ReplicaRegions(),
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	id := flag.FirstArg(ctx)

	response, err := gql.GetAddOn(ctx, client, id, string(gql.AddOnTypeUpstashRedis))
	if err != nil {
		return
	}

	addOn := response.AddOn

	excludedRegions, err := GetExcludedRegions(ctx)
	if err != nil {
		return err
	}
	excludedRegions = append(excludedRegions, addOn.PrimaryRegion)

	readRegions, err := prompt.MultiRegion(ctx, "Choose replica regions, or unselect to remove replica regions:", false, addOn.ReadRegions, excludedRegions, "replica-regions")
	if err != nil {
		return
	}

	var index int
	var promptOptions []string
	var promptDefault string

	result, err := gql.ListAddOnPlans(ctx, client, gql.AddOnTypeUpstashRedis)
	if err != nil {
		return
	}

	for _, plan := range result.AddOnPlans.Nodes {
		promptOptions = append(promptOptions, fmt.Sprintf("%s: %s", plan.DisplayName, plan.Description))
		if addOn.AddOnPlan.Id == plan.Id {
			promptDefault = fmt.Sprintf("%s: %s", plan.DisplayName, plan.Description)
		}
	}

	err = prompt.Select(ctx, &index, "Select an Upstash Redis plan", promptDefault, promptOptions...)

	if err != nil {
		return fmt.Errorf("failed to select a plan: %w", err)
	}

	// type Options struct {
	// 	Eviction bool
	// }

	// options := &Options{}

	options, _ := addOn.Options.(map[string]interface{})

	if options == nil {
		options = make(map[string]interface{})
	}

	metadata, _ := addOn.Metadata.(map[string]interface{})

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if err != nil {
		return
	}

	if options["eviction"] != nil && options["eviction"].(bool) {
		if disableEviction, err := prompt.Confirm(ctx, " Would you like to disable eviction?"); disableEviction || err != nil {
			options["eviction"] = false
		}
	} else {
		options["eviction"], err = prompt.Confirm(ctx, " Would you like to enable eviction?")
	}

	if err != nil {
		return
	}

	readRegionCodes := []string{}

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, result.AddOnPlans.Nodes[index].Id, readRegionCodes, options, metadata)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Upstash Redis database %s was updated.\n", addOn.Name)

	return
}
