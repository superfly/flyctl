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

	// Check if current plan is a legacy plan
	currentPlanIsLegacy := isLegacyPlan(addOn.AddOnPlan.DisplayName)

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
	var filteredPlans []gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan

	result, err := gql.ListAddOnPlans(ctx, client, gql.AddOnTypeUpstashRedis)
	if err != nil {
		return
	}

	// Filter plans based on current plan type
	for _, plan := range result.AddOnPlans.Nodes {
		isLegacy := isLegacyPlan(plan.DisplayName)

		// Include plan if:
		// 1. It's not a legacy plan (always include new plans), OR
		// 2. It's the current plan (so user can stay on their legacy plan)
		if !isLegacy || addOn.AddOnPlan.Id == plan.Id {
			filteredPlans = append(filteredPlans, plan)
			promptOptions = append(promptOptions, fmt.Sprintf("%s: %s", plan.DisplayName, plan.Description))
			if addOn.AddOnPlan.Id == plan.Id {
				promptDefault = fmt.Sprintf("%s: %s", plan.DisplayName, plan.Description)
			}
		}
	}

	err = prompt.Select(ctx, &index, "Select an Upstash Redis plan", promptDefault, promptOptions...)

	if err != nil {
		return fmt.Errorf("failed to select a plan: %w", err)
	}

	selectedPlan := filteredPlans[index]
	selectedPlanIsLegacy := isLegacyPlan(selectedPlan.DisplayName)
	selectedPlanIsFixed := isFixedPlan(selectedPlan.DisplayName)

	options, _ := addOn.Options.(map[string]interface{})

	if options == nil {
		options = make(map[string]interface{})
	}

	metadata, _ := addOn.Metadata.(map[string]interface{})

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// Eviction prompt (always available)
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

	// Auto-upgrade only available for fixed plans (not pay-as-you-go or legacy)
	if selectedPlanIsFixed {
		currentAutoUpgrade := false
		if options["auto_upgrade"] != nil {
			currentAutoUpgrade, _ = options["auto_upgrade"].(bool)
		}

		if currentAutoUpgrade {
			if disableAutoUpgrade, err := prompt.Confirm(ctx, "Would you like to disable auto-upgrade?"); disableAutoUpgrade || err != nil {
				options["auto_upgrade"] = false
			}
		} else {
			options["auto_upgrade"], err = prompt.Confirm(ctx, "Would you like to enable auto-upgrade?")
		}

		if err != nil {
			return
		}
	} else if !selectedPlanIsLegacy {
		// Pay-as-you-go plan - auto-upgrade not available but we should clear it if it was set
		if options["auto_upgrade"] != nil {
			delete(options, "auto_upgrade")
		}
	}

	// ProdPack available for both pay-as-you-go and fixed plans (but not legacy)
	if !selectedPlanIsLegacy {
		currentProdPack := false
		if options["prod_pack"] != nil {
			currentProdPack, _ = options["prod_pack"].(bool)
		}

		if currentProdPack {
			if disableProdPack, err := prompt.Confirm(ctx, "Would you like to disable ProdPack?"); disableProdPack || err != nil {
				options["prod_pack"] = false
			}
		} else {
			options["prod_pack"], err = prompt.Confirm(ctx, "Would you like to enable ProdPack ($200/mo)?")
		}

		if err != nil {
			return
		}
	} else if currentPlanIsLegacy {
		fmt.Fprintf(out, "\nNote: Auto-upgrade and ProdPack are not available for legacy plans.\nTo access these features, please upgrade to a current plan.\n\n")
	}

	readRegionCodes := []string{}

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, selectedPlan.Id, readRegionCodes, options, metadata)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Upstash Redis database %s was updated.\n", addOn.Name)

	return
}
