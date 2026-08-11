package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func regionsTestContext(jsonOutput bool) (context.Context, *bytes.Buffer) {
	ios, _, out, _ := iostreams.Test()

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(ctx context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{
				Regions: []fly.Region{
					{
						Code:         "iad",
						Name:         "Ashburn, Virginia (US)",
						GeoRegion:    "north_america",
						MPGAvailable: true,
					},
					{
						Code:      "arn",
						Name:      "Stockholm, Sweden",
						GeoRegion: "europe",
					},
					{
						Code:       "atl",
						Name:       "Atlanta, Georgia (US)",
						GeoRegion:  "north_america",
						Deprecated: true,
					},
				},
			}, nil
		},
	})

	return ctx, out
}

func TestRunRegionsTable(t *testing.T) {
	ctx, out := regionsTestContext(false)

	require.NoError(t, runRegions(ctx))

	output := out.String()
	require.Contains(t, output, "MPG")
	require.NotContains(t, output, "Atlanta")

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.Contains(line, "iad"):
			require.Contains(t, line, "✓")
		case strings.Contains(line, "arn"):
			require.NotContains(t, line, "✓")
		}
	}
}

func TestRunRegionsJSON(t *testing.T) {
	ctx, out := regionsTestContext(true)

	require.NoError(t, runRegions(ctx))

	var regions []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &regions))
	require.Len(t, regions, 2)

	byCode := map[string]map[string]any{}
	for _, r := range regions {
		byCode[r["code"].(string)] = r
	}

	require.Equal(t, true, byCode["iad"]["mpg_available"])
	require.Equal(t, false, byCode["arn"]["mpg_available"])
}
