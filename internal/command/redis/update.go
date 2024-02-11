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
		long = `Update an Upstash Redis database settings, payment plan or replica regions`

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
		client = client.FromContext(ctx).API().GenqClient
	)

	id := flag.FirstArg(ctx)

	response, err := gql.GetAddOn(ctx, client, id)
	if err != nil {
		return
	}

	addOn := response.AddOn

	excludedRegions, err := GetExcludedRegions(ctx)
	if err != nil {
		return err
	}
	excludedRegions = append(excludedRegions, addOn.PrimaryRegion)

	readRegions, err := prompt.MultiRegion(ctx, "Choose replica regions, or unselect to remove replica regions:", !addOn.Organization.PaidPlan, addOn.ReadRegions, excludedRegions, "replica-regions")
	if err != nil {
		return
	}

	options, _ := addOn.Options.(map[string]interface{})

	plan := addOn.AddOnPlan.Id

	if addOn.AddOnPlan.Id == redisPlanFree {
		if upgradePlan, err := prompt.Confirm(ctx, "Would you like to upgrade to the unrestricted Pay-as-you-go plan?"); upgradePlan || err != nil {
			plan = redisPlanPayAsYouGo
		}
	}

	if err != nil {
		return
	}

	if options["eviction"] != nil && options["eviction"].(bool) {
		if disableEviction, err := prompt.Confirm(ctx, "Would you like to disable eviction?"); disableEviction || err != nil {
			options["eviction"] = false
		}
	} else {
		options["eviction"], err = prompt.Confirm(ctx, "Would you like to enable eviction?")
	}

	if err != nil {
		return
	}

	readRegionCodes := []string{}

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, plan, readRegionCodes, options)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Upstash Redis database %s was updated.\n", addOn.Name)

	return
}
