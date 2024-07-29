package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

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
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

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

	var enableEviction bool = false

	if flag.GetBool(ctx, "enable-eviction") {
		enableEviction = true
	} else if !flag.GetBool(ctx, "disable-eviction") {
		fmt.Fprintf(io.Out, "\nUpstash Redis can evict objects when memory is full. This is useful when caching in Redis. This setting can be changed later.\nLearn more at https://fly.io/docs/reference/redis/#memory-limits-and-object-eviction-policies\n\n")

		enableEviction, err = prompt.Confirm(ctx, "Would you like to enable eviction?")
		if err != nil {
			return
		}
	}
	_, err = Create(ctx, org, name, primaryRegion, flag.GetBool(ctx, "no-replicas"), enableEviction, nil)
	return err
}

func Create(ctx context.Context, org *fly.Organization, name string, region *fly.Region, disallowReplicas bool, enableEviction bool, readRegions *[]fly.Region) (addOn *gql.AddOn, err error) {
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

	plan, err := DeterminePlan(ctx, org)
	if err != nil {
		return nil, err
	}

	s := spinner.Run(io, "Launching...")

	params := RedisConfiguration{
		Name:          name,
		PlanId:        plan.Id,
		PrimaryRegion: region,
		ReadRegions:   *readRegions,
		Eviction:      enableEviction,
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

func DeterminePlan(ctx context.Context, org *fly.Organization) (*gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan, error) {
	client := flyutil.ClientFromContext(ctx)

	planId := redisPlanPayAsYouGo

	// Now that we have the Plan ID, look up the actual plan
	allAddons, err := gql.ListAddOnPlans(ctx, client.GenqClient(), gql.AddOnTypeUpstashRedis)
	if err != nil {
		return nil, err
	}

	for _, addon := range allAddons.AddOnPlans.Nodes {
		if addon.Id == planId {
			return &addon, nil
		}
	}
	return nil, errors.New("plan not found")
}
