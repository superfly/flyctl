package cmdv1

import (
	"context"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func GetPlatformRegions(ctx context.Context) ([]fly.Region, error) {
	regionsFuture := prompt.PlatformRegions(ctx)
	regions, err := regionsFuture.Get()
	if err != nil {
		return nil, err
	}

	return regions.Regions, nil
}

// GetAvailableMPGRegions returns the list of regions available for Managed Postgres
func GetAvailableMPGRegions(ctx context.Context, orgSlug string) ([]fly.Region, error) {
	// Get platform regions
	platformRegions, err := GetPlatformRegions(ctx)
	if err != nil {
		return nil, err
	}

	mpgClient := mpgv1.ClientFromContext(ctx)

	// Try to get available MPG regions from API
	mpgRegionsResponse, err := mpgClient.ListMPGRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	return filterMPGRegions(platformRegions, mpgRegionsResponse.Data), nil
}

// IsValidMPGRegion checks if a region code is valid for Managed Postgres
func IsValidMPGRegion(ctx context.Context, orgSlug string, regionCode string) (bool, error) {
	availableRegions, err := GetAvailableMPGRegions(ctx, orgSlug)
	if err != nil {
		return false, err
	}

	for _, region := range availableRegions {
		if region.Code == regionCode {
			return true, nil
		}
	}

	return false, nil
}

// GetAvailableMPGRegionCodes returns just the region codes for error messages
func GetAvailableMPGRegionCodes(ctx context.Context, orgSlug string) ([]string, error) {
	availableRegions, err := GetAvailableMPGRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	var codes []string
	for _, region := range availableRegions {
		codes = append(codes, region.Code)
	}

	return codes, nil
}

// filterMPGRegions filters platform regions based on MPG availability
func filterMPGRegions(platformRegions []fly.Region, mpgRegions []mpgv1.MPGRegion) []fly.Region {
	var filteredRegions []fly.Region

	for _, region := range platformRegions {
		for _, allowed := range mpgRegions {
			if region.Code == allowed.Code && allowed.Available {
				filteredRegions = append(filteredRegions, region)

				break
			}
		}
	}

	return filteredRegions
}
