package launch

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func getRegionByCode(ctx context.Context, regionCode string) (*api.Region, error) {
	apiClient := client.FromContext(ctx).API()

	allRegions, _, err := apiClient.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range allRegions {
		if r.Code == regionCode {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("Unknown region '%s'. Run `fly platform regions` to see valid names", regionCode)
}

// computeRegionToUse looks at --region flag, existing fly.toml primary_region and as last
// meassure asks the user from a list of valid platform regions which one to use
func computeRegionToUse(ctx context.Context, appConfig *appconfig.Config, paidPlan bool) (*api.Region, error) {
	regionCode := flag.GetRegion(ctx)
	if regionCode == "" {
		regionCode = appConfig.PrimaryRegion
	}

	if regionCode != "" {
		return getRegionByCode(ctx, regionCode)
	}

	return prompt.Region(ctx, !paidPlan,
		prompt.RegionParams{Message: "Choose a region for deployment:"},
	)
}
