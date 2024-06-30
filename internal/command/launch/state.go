package launch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

// Let's *try* to keep this struct backwards-compatible as we change it
type launchPlanSource struct {
	appNameSource  string
	regionSource   string
	orgSource      string
	computeSource  string
	postgresSource string
	redisSource    string
	tigrisSource   string
	sentrySource   string
}

type LaunchManifest struct {
	Plan       *plan.LaunchPlan
	PlanSource *launchPlanSource
}

type launchState struct {
	workingDir string
	configPath string
	LaunchManifest
	env map[string]string
	planBuildCache
	cache map[string]interface{}
}

func cacheGrab[T any](cache map[string]interface{}, key string, cb func() (T, error)) (T, error) {
	if val, ok := cache[key]; ok {
		return val.(T), nil
	}
	val, err := cb()
	if err != nil {
		return val, err
	}
	cache[key] = val
	return val, nil
}

func (state *launchState) Org(ctx context.Context) (*fly.Organization, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	return cacheGrab(state.cache, "org,"+state.Plan.OrgSlug, func() (*fly.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, state.Plan.OrgSlug)
	})
}

func (state *launchState) Region(ctx context.Context) (fly.Region, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	regions, err := cacheGrab(state.cache, "regions", func() ([]fly.Region, error) {
		regions, _, err := apiClient.PlatformRegions(ctx)
		if err != nil {
			return nil, err
		}
		return regions, nil
	})
	if err != nil {
		return fly.Region{}, err
	}

	region, ok := lo.Find(regions, func(r fly.Region) bool {
		return r.Code == state.Plan.RegionCode
	})
	if !ok {
		return region, fmt.Errorf("region %s not found. Is this a valid region according to `fly platform regions`?", state.Plan.RegionCode)
	}
	return region, nil
}

// PlanSummary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (state *launchState) PlanSummary(ctx context.Context) (string, error) {
	// It feels wrong to modify the appConfig here, but in well-formed states these should be identical anyway.
	state.appConfig.Compute = state.Plan.Compute

	// Expensive but should accurately simulate the whole machine building path, meaning we end up with the same
	// guest description that will be deployed down the road :)
	fakeMachine, err := state.appConfig.ToMachineConfig(state.appConfig.DefaultProcessName(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to resolve machine guest config: %w", err)
	}
	guestStr := fakeMachine.Guest.String()

	if len(state.appConfig.Compute) > 1 {
		guestStr += fmt.Sprintf(", %d more", len(state.appConfig.Compute)-1)
	}

	org, err := state.Org(ctx)
	if err != nil {
		return "", err
	}

	region, err := state.Region(ctx)
	if err != nil {
		return "", err
	}

	postgresStr, err := describePostgresPlan(state.Plan)
	if err != nil {
		return "", err
	}

	redisStr, err := describeRedisPlan(ctx, state.Plan.Redis, org)
	if err != nil {
		return "", err
	}

	tigrisStr, err := describeObjectStoragePlan(state.Plan.ObjectStorage)
	if err != nil {
		return "", err
	}

	rows := [][]string{
		{"Organization", org.Name, state.PlanSource.orgSource},
		{"Name", state.Plan.AppName, state.PlanSource.appNameSource},
		{"Region", region.Name, state.PlanSource.regionSource},
		{"App Machines", guestStr, state.PlanSource.computeSource},
		{"Postgres", postgresStr, state.PlanSource.postgresSource},
		{"Redis", redisStr, state.PlanSource.redisSource},
		{"Tigris", tigrisStr, state.PlanSource.tigrisSource},
	}

	if state.PlanSource.sentrySource != "not requested" {
		rows = append(rows, []string{"Sentry", strconv.FormatBool(state.Plan.Sentry), state.PlanSource.sentrySource})
	}

	for _, row := range rows {
		// TODO: This is a hack. It'd be nice to not require a special sentinel value for the description,
		//       but it works OK for now. I'd special-case on value=="" instead, but that isn't *necessarily*
		//       a failure case for every field.
		if row[2] == recoverableSpecifyInUi {
			row[1] = "<unspecified>"
		}
	}

	colLengths := []int{0, 0, 0}
	for _, row := range rows {
		for i, col := range row {
			if len(col) > colLengths[i] {
				colLengths[i] = len(col)
			}
		}
	}

	ret := ""
	for _, row := range rows {

		label := row[0]
		value := row[1]
		source := row[2]

		labelSpaces := strings.Repeat(" ", colLengths[0]-len(label))
		valueSpaces := strings.Repeat(" ", colLengths[1]-len(value))

		ret += fmt.Sprintf("%s: %s%s %s(%s)\n", label, labelSpaces, value, valueSpaces, source)
	}
	return ret, nil
}

func (state *launchState) validateExtensions(ctx context.Context) error {
	// This is written a little awkwardly with the expectation
	// that we'll probably need more validation in the future.
	// When that happens we can just errors.Join(a(), b(), c()...)

	io := iostreams.FromContext(ctx)
	noConfirm := !io.IsInteractive() || flag.GetBool(ctx, "now")

	org, err := state.Org(ctx)
	if err != nil {
		return err
	}

	validateSupabase := func() error {
		supabase := state.Plan.Postgres.SupabasePostgres
		if supabase == nil {
			return nil
		}

		// We're using Supabase. Ensure that we're within plan limits.
		client := flyutil.ClientFromContext(ctx).GenqClient()

		response, err := gql.ListAddOns(ctx, client, "supabase")
		if err != nil {
			return fmt.Errorf("failed to list Supabase databases: %w", err)
		}

		// TODO: We'd like to be able to query the user's plan to see if they're on a paid plan.
		//       For now, we'll just nag when they create their second database, every time.

		if len(response.AddOns.Nodes) != 1 {
			// If we're at zero databases, we're within the free plan.
			// If we're at >=2 databases, we know we're on a paid plan.
			// It's only 1 existing database where we need to validate the plan.
			return nil
		}

		if noConfirm {
			// We can't validate this any further until we can query the plan info.
			// Assume it's okay, and let the launch fail if it's not.

			// TODO: Once we can query whether or not the user is on a paid plan,
			//       we'll be able to early-exit in non-interactive mode and prevent a failed launch.
			return nil
		}

		fmt.Fprintf(io.Out, "You're about to create a second Supabase database. This requires a paid plan.\n")
		fmt.Fprintf(io.Out, "Please check to ensure that your plan supports this, otherwise your launch may fail.\n")
		openDashboard, err := prompt.Confirm(ctx, "Open the dashboard to check your plan?")
		if err != nil {
			return err
		}
		if openDashboard {
			if err = extensions_core.OpenOrgDashboard(ctx, org.Slug, "supabase"); err != nil {
				return err
			}
		}
		confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Continue launching %s?", state.Plan.AppName))
		if err != nil {
			return err
		}
		if !confirm {
			return errors.New("aborted by user")
		}

		return nil
	}

	return validateSupabase()
}
