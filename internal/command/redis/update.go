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
		flag.Bool{
			Name:        "enable-prodpack",
			Description: "Enable ProdPack add-on for additional features ($200/mo)",
		},
		flag.Bool{
			Name:        "disable-prodpack",
			Description: "Disable the ProdPack add-on",
		},
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

	options, _ := addOn.Options.(map[string]any)

	if options == nil {
		options = make(map[string]any)
	}

	metadata, _ := addOn.Metadata.(map[string]any)

	if metadata == nil {
		metadata = make(map[string]any)
	}

	// Eviction prompt (always available)
	if options["eviction"] != nil && options["eviction"].(bool) {
		disableEviction, err := prompt.Confirm(ctx, "Would you like to disable eviction?")
		if err != nil {
			return err
		}
		if disableEviction {
			options["eviction"] = false
		}
	} else {
		enableEviction, err := prompt.Confirm(ctx, "Would you like to enable eviction?")
		if err != nil {
			return err
		}
		if enableEviction {
			options["eviction"] = true
		} else {
			// Declining to enable must not send an explicit false: if the
			// add-on metadata is stale, that would disable a feature that is
			// actually enabled. Omit the key so the server keeps its state.
			delete(options, "eviction")
		}
	}

	// Auto-upgrade only available for fixed plans (not pay-as-you-go or legacy)
	if selectedPlanIsFixed {
		currentAutoUpgrade := false
		if options["auto_upgrade"] != nil {
			currentAutoUpgrade, _ = options["auto_upgrade"].(bool)
		}

		if currentAutoUpgrade {
			disableAutoUpgrade, err := prompt.Confirm(ctx, "Would you like to disable auto-upgrade?")
			if err != nil {
				return err
			}
			if disableAutoUpgrade {
				options["auto_upgrade"] = false
			}
		} else {
			enableAutoUpgrade, err := prompt.Confirm(ctx, "Would you like to enable auto-upgrade?")
			if err != nil {
				return err
			}
			if enableAutoUpgrade {
				options["auto_upgrade"] = true
			} else {
				delete(options, "auto_upgrade")
			}
		}
	} else if !selectedPlanIsLegacy {
		// Pay-as-you-go plan - auto-upgrade not available but we should clear it if it was set
		if options["auto_upgrade"] != nil {
			delete(options, "auto_upgrade")
		}
	}

	// ProdPack available for both pay-as-you-go and fixed plans (but not legacy).
	//
	// The locally stored prod_pack option can be stale in either direction, so
	// the update payload only ever carries a value the user chose in this
	// invocation; otherwise the key is omitted and the provider preserves the
	// database's current state. Deriving the sent value from the stored copy is
	// what allowed an ordinary plan change to disable an active ProdPack.
	prodPack, err := resolveProdPack(
		flag.GetBool(ctx, "enable-prodpack"),
		flag.GetBool(ctx, "disable-prodpack"),
		options["prod_pack"],
		selectedPlanIsLegacy,
		func() (bool, error) {
			return prompt.Confirm(ctx, "Would you like to disable ProdPack?")
		},
	)
	if err != nil {
		return
	}

	if err = validateProdPackPlanChange(prodPack, addOn.AddOnPlan.Id, selectedPlan.Id); err != nil {
		return
	}

	stripProdPack(options)

	if prodPack == nil && !selectedPlanIsLegacy {
		fmt.Fprintf(out, "ProdPack: unchanged (use --enable-prodpack or --disable-prodpack to change it)\n")
	}

	if currentPlanIsLegacy && selectedPlanIsLegacy {
		fmt.Fprintf(out, "\nNote: Auto-upgrade and ProdPack are not available for legacy plans.\nTo access these features, please upgrade to a current plan.\n\n")
	}

	readRegionCodes := []string{}

	for _, region := range *readRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	_, err = gql.UpdateRedisAddOn(ctx, client, addOn.Id, selectedPlan.Id, readRegionCodes, options, metadata, prodPack)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Upstash Redis database %s was updated.\n", addOn.Name)

	return
}

// resolveProdPack determines the user's explicit ProdPack decision for this
// invocation. A nil result means no decision was made and the prod_pack key
// must be omitted from the update payload.
//
// The stored option is intentionally never echoed back as the sent value: it
// is only used to decide whether offering the disable prompt makes sense. A
// non-interactive session makes no decision rather than defaulting to disable.
func resolveProdPack(enableFlag, disableFlag bool, stored any, selectedPlanIsLegacy bool, confirmDisable func() (bool, error)) (*bool, error) {
	if enableFlag && disableFlag {
		return nil, fmt.Errorf("--enable-prodpack and --disable-prodpack are mutually exclusive")
	}

	if selectedPlanIsLegacy {
		if enableFlag {
			return nil, fmt.Errorf("ProdPack is not available for legacy plans")
		}
		if disableFlag {
			return boolPtr(false), nil
		}

		return nil, nil
	}

	if enableFlag {
		return boolPtr(true), nil
	}
	if disableFlag {
		return boolPtr(false), nil
	}

	// Only offer the disable prompt when the stored copy claims ProdPack is
	// on. Never prompt to enable: a default/accidental answer previously
	// became an explicit disable for databases whose stored option was stale.
	if storedOn, _ := stored.(bool); storedOn {
		disable, err := confirmDisable()
		switch {
		case prompt.IsNonInteractive(err):
			return nil, nil
		case err != nil:
			return nil, err
		case disable:
			return boolPtr(false), nil
		}
	}

	return nil, nil
}

// stripProdPack removes the legacy ProdPack option from the options blob.
// ProdPack intent travels only through the typed prodPack GraphQL argument;
// the provider must never receive it in the legacy options map.
func stripProdPack(options map[string]any) {
	delete(options, "prod_pack")
}

func boolPtr(v bool) *bool {
	return &v
}

func validateProdPackPlanChange(decision *bool, currentPlanID, selectedPlanID string) error {
	if decision != nil && currentPlanID == selectedPlanID {
		return fmt.Errorf("ProdPack can only be changed together with a plan change; pick a different plan or drop the flag")
	}

	return nil
}
