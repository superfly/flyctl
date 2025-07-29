package scale

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_convergeGroupCounts(t *testing.T) {
	testcases := []struct {
		name          string
		want          map[string]int
		expectedTotal int
		current       map[string]int
		regions       []string
		maxPerRegion  int
	}{
		{
			name:          "Spread instances across regions from nothing",
			want:          map[string]int{"scl": 2, "iad": 1},
			expectedTotal: 3,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Spread instances across regions from existing",
			want:          map[string]int{"scl": 1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 3,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Act on all current regions if not region is passed",
			want:          map[string]int{"scl": 2, "iad": 2},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 6,
			maxPerRegion:  -1,
		},
		{
			name:          "Requirements already met",
			want:          map[string]int{},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 2,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Reduce the fleet",
			want:          map[string]int{"iad": -1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 1,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Reduce the fleet (like previous but order of regions matters)",
			want:          map[string]int{"scl": -1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 1,
			regions:       []string{"iad", "scl"},
			maxPerRegion:  -1,
		},
		// Ignore non-listed regions
		{
			name:          "Ignore non-listed regions while removing machines",
			want:          map[string]int{"scl": -3, "iad": -3},
			current:       map[string]int{"scl": 3, "iad": 5, "ord": 1, "sin": 10},
			expectedTotal: 2,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Ignore non-listed regions while adding machines",
			want:          map[string]int{"scl": 2, "iad": 2},
			current:       map[string]int{"scl": 3, "iad": 5, "ord": 1, "sin": 10},
			expectedTotal: 12,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convergeGroupCounts(tc.expectedTotal, tc.current, tc.regions, tc.maxPerRegion)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_convergeGroupCounts_maxPerRegion(t *testing.T) {
	// maxPerRegion * len(regions) < expectedTotal must fail
	_, err := convergeGroupCounts(10, nil, []string{"scl", "mia"}, 1)
	assert.Equal(t, MaxPerRegionError, err)

	// Happy path cases
	testcases := []struct {
		name          string
		want          map[string]int
		expectedTotal int
		current       map[string]int
		regions       []string
		maxPerRegion  int
	}{
		{
			name:          "Spread instances across regions respecting max per region",
			want:          map[string]int{"scl": 1, "iad": 2},
			current:       map[string]int{"scl": 2, "iad": 1},
			expectedTotal: 6,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  3,
		},
		{
			name:          "Spread instances across regions respecting max per regioni with reductions",
			want:          map[string]int{"scl": -5, "iad": 1, "ord": 2, "sin": 2},
			current:       map[string]int{"scl": 7, "iad": 1},
			expectedTotal: 8,
			regions:       []string{"scl", "iad", "ord", "sin"},
			maxPerRegion:  2,
		},
		{
			name:          "Spread respecting unlisted regions",
			want:          map[string]int{"scl": -5, "iad": 1, "ord": 2, "sin": 2},
			current:       map[string]int{"scl": 7, "iad": 1, "mia": 10},
			expectedTotal: 8,
			regions:       []string{"scl", "iad", "ord", "sin"},
			maxPerRegion:  2,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convergeGroupCounts(tc.expectedTotal, tc.current, tc.regions, tc.maxPerRegion)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
