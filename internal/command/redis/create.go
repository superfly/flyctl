package redis

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/spinner"
)

const (
	redisPlanFree       = "x7M0gyB764ggwt6YZLRK"
	redisPlanPayAsYouGo = "ekQ85Yjkw155ohQ5ALYq0M"
)

// Legacy plans that are no longer available for new databases
// but existing databases can remain on them
// These match the normalized display names (lowercase, spaces replaced with underscores)
var legacyPlans = []string{
	"pro_2k",   // "Pro 2k"
	"pro_10k",  // "Pro 10k"
	"starter",  // "Starter"
	"standard", // "Standard"
}

func isLegacyPlan(planName string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(planName, " ", "_"))
	return slices.Contains(legacyPlans, normalized)
}

// isFixedPlan checks if a plan is one of the new fixed plans (not pay-as-you-go)
// Auto-upgrade is only available for fixed plans
func isFixedPlan(planName string) bool {
	return strings.HasPrefix(strings.ToLower(planName), "flyio_fixed_") ||
		strings.HasPrefix(strings.ToLower(planName), "fixed ")
}

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
		flag.ReplicaRegions(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Redis database",
		},
		flag.String{
			Name:        "plan",
			Description: "The plan for your Redis database (default: pay-as-you-go)",
		},
		flag.Bool{
			Name:        "no-replicas",
			Description: "Don't prompt for selecting replica regions",
		},
		flag.Bool{
			Name:        "enable-eviction",
			Description: "Evict objects when memory is full",
		},
		flag.Bool{
			Name:        "disable-eviction",
			Description: "Disallow writes when the max data size limit has been reached",
		},
		flag.Bool{
			Name:        "enable-auto-upgrade",
			Description: "Automatically upgrade to a higher plan when hitting resource limits",
		},
		flag.Bool{
			Name:        "enable-prodpack",
			Description: "Enable ProdPack add-on for additional features ($200/mo)",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	// Validate --plan flag early before prompting for other options
	planName := flag.GetString(ctx, "plan")
	if planName != "" && isLegacyPlan(planName) {
		return fmt.Errorf("plan %q is no longer available for new databases. Please choose a current plan", planName)
	}

	// pre-fetch platform regions for later use
	prompt.PlatformRegions(ctx)

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	name := flag.GetString(ctx, "name")

	if name == "" {
		err = prompt.String(ctx, &name, "Choose a Redis database name (leave blank to generate one):", "", false)

		if err != nil {
			return err
		}
	}

	excludedRegions, err := GetExcludedRegions(ctx)
	if err != nil {
		return err
	}

	primaryRegion, err := prompt.Region(ctx, false, prompt.RegionParams{
		Message:             "Choose a primary region (can't be changed later)",
		ExcludedRegionCodes: excludedRegions,
	})
	if err != nil {
		return err
	}

	var enableEviction = false

	if flag.GetBool(ctx, "enable-eviction") {
		enableEviction = true
	} else if !flag.GetBool(ctx, "disable-eviction") {
		fmt.Fprintf(io.Out, "\nUpstash Redis can evict objects when memory is full. This is useful when caching in Redis. This setting can be changed later.\nLearn more at https://fly.io/docs/reference/redis/#memory-limits-and-object-eviction-policies\n\n")

		enableEviction, err = prompt.Confirm(ctx, "Would you like to enable eviction?")
		if err != nil {
			return
		}
	}

	// Determine plan (already validated above if --plan was specified)
	plan, err := DeterminePlan(ctx, planName)
	if err != nil {
		return err
	}

	// Check if the selected plan is a fixed plan
	planIsFixed := isFixedPlan(plan.DisplayName)

	// Prompt for auto-upgrade option (fixed plans only)
	var enableAutoUpgrade bool
	if planIsFixed {
		if flag.IsSpecified(ctx, "enable-auto-upgrade") {
			enableAutoUpgrade = flag.GetBool(ctx, "enable-auto-upgrade")
		} else {
			fmt.Fprintf(io.Out, "\nAuto-upgrade automatically switches to a higher plan when you hit resource limits.\nThis setting can be changed later.\n\n")
			enableAutoUpgrade, err = prompt.Confirm(ctx, "Would you like to enable auto-upgrade?")
			if err != nil {
				return
			}
		}
	} else if flag.IsSpecified(ctx, "enable-auto-upgrade") && flag.GetBool(ctx, "enable-auto-upgrade") {
		fmt.Fprintf(io.Out, "\nNote: Auto-upgrade is only available for fixed plans, not pay-as-you-go.\n")
	}

	// prompt for prodpack option (pay-as-you-go and fixed plans)
	var enableProdpack bool
	if flag.IsSpecified(ctx, "enable-prodpack") {
		enableProdpack = flag.GetBool(ctx, "enable-prodpack")
	} else {
		fmt.Fprintf(io.Out, "\nProdPack adds enhanced features for production workloads at $200/mo.\nThis setting can be changed later.\n\n")
		enableProdpack, err = prompt.Confirm(ctx, "Would you like to enable ProdPack?")
		if err != nil {
			return
		}
	}

	_, err = Create(ctx, org, name, primaryRegion, plan, flag.GetBool(ctx, "no-replicas"), enableEviction, enableAutoUpgrade, enableProdpack, nil)
	return err
}

func Create(ctx context.Context, org *fly.Organization, name string, region *fly.Region, plan *gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan, disallowReplicas bool, enableEviction bool, enableAutoUpgrade bool, enableProdpack bool, readRegions *[]fly.Region) (addOn *gql.AddOn, err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	excludedRegions, err := GetExcludedRegions(ctx)
	if err != nil {
		return nil, err
	}

	excludedRegions = append(excludedRegions, region.Code)

	if readRegions == nil {
		readRegions = &[]fly.Region{}

		if !disallowReplicas {
			readRegions, err = prompt.MultiRegion(ctx, "Optionally, choose one or more replica regions (can be changed later):", false, []string{}, excludedRegions, "replica-regions")

			if err != nil {
				return
			}
		}
	} else {
		// Validate that the read regions are not in the excluded regions
		var invalidRegions []string
		for _, readRegion := range *readRegions {
			if slices.Contains(excludedRegions, readRegion.Code) {
				invalidRegions = append(invalidRegions, readRegion.Code)
			}
		}
		if len(invalidRegions) > 0 {
			return nil, fmt.Errorf("invalid replica regions: %v", invalidRegions)
		}
	}

	s := spinner.Run(io, "Launching...")

	params := RedisConfiguration{
		Name:          name,
		PlanId:        plan.Id,
		PrimaryRegion: region,
		ReadRegions:   *readRegions,
		Eviction:      enableEviction,
		AutoUpgrade:   enableAutoUpgrade,
		ProdPack:      enableProdpack,
	}

	addOn, err = ProvisionDatabase(ctx, org, params)

	s.Stop()
	if err != nil {
		return
	}

	fmt.Fprintf(io.Out, "\nYour database %s is ready. Apps in the %s org can connect to Redis at %s\n", colorize.Green(addOn.Name), colorize.Green(org.Slug), colorize.Green(addOn.PublicUrl))
	fmt.Fprintf(io.Out, "\nIf you have redis-cli installed, use %s to get a Redis console.\n", colorize.Green("fly redis connect"))
	fmt.Fprintf(io.Out, "\nYour database is billed at %s. If you're using Sidekiq or BullMQ, which poll Redis frequently, consider switching to a fixed-price plan. See https://fly.io/docs/reference/redis/#pricing\n", colorize.Green("$0.20 per 100K commands"))

	return addOn, err
}

type RedisConfiguration struct {
	Name          string
	PlanId        string
	PrimaryRegion *fly.Region
	ReadRegions   []fly.Region
	Eviction      bool
	AutoUpgrade   bool
	ProdPack      bool
}

func ProvisionDatabase(ctx context.Context, org *fly.Organization, config RedisConfiguration) (addOn *gql.AddOn, err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	var readRegionCodes []string

	for _, region := range config.ReadRegions {
		readRegionCodes = append(readRegionCodes, region.Code)
	}

	options := gql.AddOnOptions{}

	if config.Eviction {
		options["eviction"] = true
	}
	if config.AutoUpgrade {
		options["auto_upgrade"] = true
	}
	if config.ProdPack {
		options["prod_pack"] = true
	}

	input := gql.CreateAddOnInput{
		OrganizationId: org.ID,
		PrimaryRegion:  config.PrimaryRegion.Code,
		Name:           config.Name,
		PlanId:         config.PlanId,
		ReadRegions:    readRegionCodes,
		Type:           "upstash_redis",
		Options:        options,
	}

	response, err := gql.CreateAddOn(ctx, client, input)
	if err != nil {
		return
	}

	return &response.CreateAddOn.AddOn, nil
}

func DeterminePlan(ctx context.Context, planName string) (*gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan, error) {
	client := flyutil.ClientFromContext(ctx)

	// Fetch all available plans
	allPlans, err := gql.ListAddOnPlans(ctx, client.GenqClient(), gql.AddOnTypeUpstashRedis)
	if err != nil {
		return nil, err
	}

	// If a specific plan is requested, use it if it's not a legacy plan
	if planName != "" {
		if isLegacyPlan(planName) {
			return nil, fmt.Errorf("plan %q is no longer available for new databases. Please choose a current plan", planName)
		}
		for _, plan := range allPlans.AddOnPlans.Nodes {
			if plan.DisplayName == planName || plan.Id == planName {
				return &plan, nil
			}
		}
		return nil, fmt.Errorf("plan %q not found", planName)
	}

	// Default to pay-as-you-go plan
	for _, plan := range allPlans.AddOnPlans.Nodes {
		if plan.Id == redisPlanPayAsYouGo {
			return &plan, nil
		}
	}
	return nil, errors.New("default plan not found")
}
