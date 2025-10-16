package curl

import (
	"context"
	"testing"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockFlyClient implements the necessary methods for testing
type MockFlyClient struct {
	*fly.Client
	PlatformRegionsFunc func(ctx context.Context) ([]fly.Region, *fly.Region, error)
}

func (m *MockFlyClient) PlatformRegions(ctx context.Context) ([]fly.Region, *fly.Region, error) {
	if m.PlatformRegionsFunc != nil {
		return m.PlatformRegionsFunc(ctx)
	}
	return []fly.Region{}, nil, nil
}

func TestFetchRegionCodes_FiltersDeprecated(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "ord", Name: "Chicago", Deprecated: false},
		{Code: "lax", Name: "Los Angeles", Deprecated: false},
		{Code: "atl", Name: "Atlanta", Deprecated: true},
		{Code: "ams", Name: "Amsterdam", Deprecated: false},
		{Code: "hkg", Name: "Hong Kong", Deprecated: true},
		{Code: "fra", Name: "Frankfurt", Deprecated: false},
	}

	mockClient := &MockFlyClient{
		PlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, *fly.Region, error) {
			return testRegions, nil, nil
		},
	}

	ctx := context.Background()
	ctx = flyutil.NewContextWithClient(ctx, mockClient)

	codes, err := fetchRegionCodes(ctx)

	require.NoError(t, err)
	assert.Len(t, codes, 4, "Should return 4 non-deprecated region codes")

	// Verify the specific regions are included
	assert.Contains(t, codes, "ord")
	assert.Contains(t, codes, "lax")
	assert.Contains(t, codes, "ams")
	assert.Contains(t, codes, "fra")

	// Verify deprecated regions are excluded
	assert.NotContains(t, codes, "atl")
	assert.NotContains(t, codes, "hkg")

	// Verify codes are sorted
	assert.Equal(t, []string{"ams", "fra", "lax", "ord"}, codes, "Region codes should be sorted")
}

func TestFetchRegionCodes_AllDeprecated(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "atl", Name: "Atlanta", Deprecated: true},
		{Code: "hkg", Name: "Hong Kong", Deprecated: true},
	}

	mockClient := &MockFlyClient{
		PlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, *fly.Region, error) {
			return testRegions, nil, nil
		},
	}

	ctx := context.Background()
	ctx = flyutil.NewContextWithClient(ctx, mockClient)

	codes, err := fetchRegionCodes(ctx)

	require.NoError(t, err)
	assert.Len(t, codes, 0, "Should return empty list when all regions are deprecated")
}

func TestFetchRegionCodes_NoneDeprecated(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "ord", Name: "Chicago", Deprecated: false},
		{Code: "lax", Name: "Los Angeles", Deprecated: false},
		{Code: "ams", Name: "Amsterdam", Deprecated: false},
	}

	mockClient := &MockFlyClient{
		PlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, *fly.Region, error) {
			return testRegions, nil, nil
		},
	}

	ctx := context.Background()
	ctx = flyutil.NewContextWithClient(ctx, mockClient)

	codes, err := fetchRegionCodes(ctx)

	require.NoError(t, err)
	assert.Len(t, codes, 3, "Should return all region codes")
	assert.Equal(t, []string{"ams", "lax", "ord"}, codes, "Region codes should be sorted")
}
