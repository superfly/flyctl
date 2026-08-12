package mpg

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

// AvailableRegions returns the non-deprecated regions where Managed Postgres is
// available, per the Machines API region list (fly.Region.MPGAvailable).
func AvailableRegions(ctx context.Context) ([]fly.Region, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	if flapsClient == nil {
		return nil, fmt.Errorf("flaps client not found in context")
	}

	res, err := flapsClient.GetRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving regions: %w", err)
	}

	return lo.Filter(res.Regions, func(r fly.Region, _ int) bool {
		return r.MPGAvailable && !r.Deprecated
	}), nil
}

// IsValidRegion reports whether Managed Postgres is available in the given
// region code.
func IsValidRegion(ctx context.Context, regionCode string) (bool, error) {
	regions, err := AvailableRegions(ctx)
	if err != nil {
		return false, err
	}

	for _, r := range regions {
		if r.Code == regionCode {
			return true, nil
		}
	}

	return false, nil
}

// AvailableRegionCodes returns the codes of AvailableRegions.
func AvailableRegionCodes(ctx context.Context) ([]string, error) {
	regions, err := AvailableRegions(ctx)
	if err != nil {
		return nil, err
	}

	return lo.Map(regions, func(r fly.Region, _ int) string { return r.Code }), nil
}
