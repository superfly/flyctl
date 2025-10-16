package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
)

func TestDeprecatedRegionFiltering(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "ord", Name: "Chicago", Deprecated: false},
		{Code: "lax", Name: "Los Angeles", Deprecated: false},
		{Code: "atl", Name: "Atlanta", Deprecated: true},
		{Code: "ams", Name: "Amsterdam", Deprecated: false},
		{Code: "hkg", Name: "Hong Kong", Deprecated: true},
		{Code: "fra", Name: "Frankfurt", Deprecated: false},
		{Code: "bos", Name: "Boston", Deprecated: true},
	}

	// Call the actual filtering function
	filteredRegions := filterDeprecatedRegions(testRegions)

	// Verify filtering
	assert.Len(t, filteredRegions, 4, "Should filter out 3 deprecated regions")

	// Verify all returned regions are not deprecated
	for _, region := range filteredRegions {
		assert.False(t, region.Deprecated, "Region %s should not be deprecated", region.Code)
	}

	// Verify the specific regions are included
	regionCodes := make([]string, len(filteredRegions))
	for i, region := range filteredRegions {
		regionCodes[i] = region.Code
	}
	assert.Contains(t, regionCodes, "ord")
	assert.Contains(t, regionCodes, "lax")
	assert.Contains(t, regionCodes, "ams")
	assert.Contains(t, regionCodes, "fra")

	// Verify deprecated regions are excluded
	assert.NotContains(t, regionCodes, "atl")
	assert.NotContains(t, regionCodes, "hkg")
	assert.NotContains(t, regionCodes, "bos")
}

func TestDeprecatedRegionFiltering_AllDeprecated(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "atl", Name: "Atlanta", Deprecated: true},
		{Code: "hkg", Name: "Hong Kong", Deprecated: true},
		{Code: "bos", Name: "Boston", Deprecated: true},
	}

	// Call the actual filtering function
	filteredRegions := filterDeprecatedRegions(testRegions)

	// Should return empty list
	assert.Len(t, filteredRegions, 0, "Should filter out all deprecated regions")
}

func TestDeprecatedRegionFiltering_NoneDeprecated(t *testing.T) {
	testRegions := []fly.Region{
		{Code: "ord", Name: "Chicago", Deprecated: false},
		{Code: "lax", Name: "Los Angeles", Deprecated: false},
		{Code: "ams", Name: "Amsterdam", Deprecated: false},
	}

	// Call the actual filtering function
	filteredRegions := filterDeprecatedRegions(testRegions)

	// Should return all regions
	assert.Len(t, filteredRegions, 3, "Should include all non-deprecated regions")

	for _, region := range filteredRegions {
		assert.False(t, region.Deprecated, "Region %s should not be deprecated", region.Code)
	}
}

func TestDeprecatedRegionFiltering_EmptyList(t *testing.T) {
	testRegions := []fly.Region{}

	// Call the actual filtering function
	filteredRegions := filterDeprecatedRegions(testRegions)

	// Should return empty list
	assert.Len(t, filteredRegions, 0, "Should handle empty region list")
}
