package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/scanner"
)

type launchState struct {
	workingDir string
	configPath string
	plan       *launchPlan
	planSource *launchPlanSource
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
	return cacheGrab(state.cache, "org,"+state.plan.OrgSlug, func() (*api.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, state.plan.OrgSlug)
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
		return r.Code == state.plan.RegionCode
	})
	if !ok {
		return region, fmt.Errorf("region %state not found", state.plan.RegionCode)
	}
	return region, nil
}

// PlanSummary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (state *launchState) PlanSummary(ctx context.Context) (string, error) {

	guest := state.plan.Guest()

	org, err := state.Org(ctx)
	if err != nil {
		return "", err
	}

	region, err := state.Region(ctx)
	if err != nil {
		return "", err
	}

	postgresStr, err := state.plan.Postgres.Describe(ctx)
	if err != nil {
		return "", err
	}

	redisStr, err := state.plan.Redis.Describe(ctx)
	if err != nil {
		return "", err
	}

	rows := [][]string{
		{"Organization", org.Name, state.planSource.orgSource},
		{"Name", state.plan.AppName, state.planSource.appNameSource},
		{"Region", region.Name, state.planSource.regionSource},
		{"App Machines", guest.String(), state.planSource.guestSource},
		{"Postgres", postgresStr, state.planSource.postgresSource},
		{"Redis", redisStr, state.planSource.redisSource},
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
