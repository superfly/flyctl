package launch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

// Let's *try* to keep this struct backwards-compatible as we change it
type launchPlanSource struct {
	appNameSource  string
	regionSource   string
	orgSource      string
	computeSource  string
	postgresSource string
	redisSource    string
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
	apiClient := fly.ClientFromContext(ctx)
	return cacheGrab(state.cache, "org,"+state.Plan.OrgSlug, func() (*fly.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, state.Plan.OrgSlug)
	})
}

func (state *launchState) Region(ctx context.Context) (fly.Region, error) {
	apiClient := fly.ClientFromContext(ctx)
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

	postgresStr, err := describePostgresPlan(ctx, state.Plan.Postgres, org)
	if err != nil {
		return "", err
	}

	redisStr, err := describeRedisPlan(ctx, state.Plan.Redis, org)
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
		{"Sentry", strconv.FormatBool(state.Plan.Sentry), state.PlanSource.sentrySource},
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
