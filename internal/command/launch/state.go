package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/scanner"
)

// Let's *try* to keep this struct backwards-compatible as we change it
type launchPlanSource struct {
	appNameSource  string
	regionSource   string
	orgSource      string
	guestSource    string
	postgresSource string
	redisSource    string
}

type LaunchManifest struct {
	Plan       *plan.LaunchPlan
	PlanSource *launchPlanSource
}

type launchState struct {
	workingDir string
	configPath string
	LaunchManifest
	env        map[string]string
	appConfig  *appconfig.Config
	sourceInfo *scanner.SourceInfo
	cache      map[string]interface{}
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

func (state *launchState) Org(ctx context.Context) (*api.Organization, error) {
	apiClient := client.FromContext(ctx).API()
	return cacheGrab(state.cache, "org,"+state.Plan.OrgSlug, func() (*api.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, state.Plan.OrgSlug)
	})
}

func (state *launchState) Region(ctx context.Context) (api.Region, error) {

	apiClient := client.FromContext(ctx).API()
	regions, err := cacheGrab(state.cache, "regions", func() ([]api.Region, error) {
		regions, _, err := apiClient.PlatformRegions(ctx)
		if err != nil {
			return nil, err
		}
		return regions, nil
	})
	if err != nil {
		return api.Region{}, err
	}

	region, ok := lo.Find(regions, func(r api.Region) bool {
		return r.Code == state.Plan.RegionCode
	})
	if !ok {
		return region, fmt.Errorf("region %state not found", state.Plan.RegionCode)
	}
	return region, nil
}

// PlanSummary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (state *launchState) PlanSummary(ctx context.Context) (string, error) {

	guest := state.Plan.Guest()

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
		{"App Machines", guest.String(), state.PlanSource.guestSource},
		{"Postgres", postgresStr, state.PlanSource.postgresSource},
		{"Redis", redisStr, state.PlanSource.redisSource},
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
