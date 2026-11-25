package prompt

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestMultiRegionFromFlag(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []string
		wantErr     string
	}{
		{"codes", "ord,iad", []string{"iad", "ord"}, ""},
		{"spaces and trailing comma", " ord, iad,", []string{"iad", "ord"}, ""},
		{"typo", "odr", nil, "region odr not found"},
		{"typo next to a valid code", "ord,iadd", nil, "region iadd not found"},
		{"excluded region is left out", "ord,lhr", []string{"ord"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("replica-regions", tc.value, "")
			ctx := flag.NewContext(t.Context(), flags)
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
					return &flaps.RegionData{Regions: []fly.Region{
						{Code: "iad", Name: "Ashburn"}, {Code: "lhr", Name: "London"}, {Code: "ord", Name: "Chicago"},
					}}, nil
				},
			})

			regions, err := MultiRegion(ctx, "", false, nil, []string{"lhr"}, "replica-regions")
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)

				return
			}
			require.NoError(t, err)
			var codes []string
			for _, r := range *regions {
				codes = append(codes, r.Code)
			}
			require.ElementsMatch(t, tc.want, codes)
		})
	}
}
