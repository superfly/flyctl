package scale

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
)

func TestScaleCountPerRegionArgs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"absolute", []string{"4", "--per-region", "-r", "ord,sjc"}, ""},
		{"groups and zero", []string{"web=4", "worker=0", "--per-region", "-r", "ord"}, ""},
		{"missing regions", []string{"4", "--per-region"}, "requires an explicit --region"},
		{"empty region", []string{"4", "--per-region", "-r", "ord,"}, "requires an explicit --region"},
		{"spaces", []string{"4", "--per-region", "-r", "ord, sjc"}, "requires an explicit --region"},
		{"relative", []string{"+1", "--per-region", "-r", "ord"}, "requires absolute counts"},
		{"relative group", []string{"web=4", "worker=-1", "--per-region", "-r", "ord"}, "requires absolute counts"},
		{"cap", []string{"4", "--per-region", "-r", "ord", "--max-per-region", "4"}, "cannot be combined"},
		{"explicit default cap", []string{"4", "--per-region", "-r", "ord", "--max-per-region=-1"}, "cannot be combined"},
		{"invalid count", []string{"web=no", "--per-region", "-r", "ord"}, "invalid syntax"},
		{"legacy relative", []string{"+1", "-r", "ord"}, ""},
		{"legacy cap", []string{"4", "--max-per-region", "2"}, ""},
		{"explicit false", []string{"+1", "--per-region=false"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newScaleCount()
			require.NoError(t, cmd.ParseFlags(tc.args))
			err := cmd.ValidateArgs(cmd.Flags().Args())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScaleCountPerRegionPlan(t *testing.T) {
	for _, perRegion := range []bool{false, true} {
		cmd := newScaleCount()
		args := []string{"app=4", "worker=0", "-r", "ord,sjc,syd,lax,ams,ord"}
		if perRegion {
			args = append(args, "--per-region")
		}
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, cmd.ValidateArgs(cmd.Flags().Args()))
		groups, err := parseGroupCounts(cmd.Flags().Args(), "app")
		require.NoError(t, err)
		machines := countTestMachines(map[string]map[string]int{
			"app":       {"ord": 6, "sjc": 1, "syd": 2, "lax": 2, "ams": 2, "nrt": 3},
			"worker":    {"ord": 2, "nrt": 3},
			"untouched": {"ord": 2},
		})
		actions, err := computeActions("local-test", machines, groups, []string{"ord", "sjc", "syd", "lax", "ams", "ord"}, -1, &defaultValues{}, flag.GetBool(flag.NewContext(t.Context(), cmd.Flags()), "per-region"))
		require.NoError(t, err)
		got := map[string]map[string]int{}
		for _, action := range actions {
			if got[action.GroupName] == nil {
				got[action.GroupName] = map[string]int{}
			}
			require.NotContains(t, got[action.GroupName], action.Region)
			got[action.GroupName][action.Region] = action.Delta
		}
		want := map[string]int{"ord": -2, "sjc": 3, "syd": 2, "lax": 2, "ams": 2}
		if !perRegion {
			want = map[string]int{"ord": -2, "sjc": -1, "syd": -2, "lax": -2, "ams": -2}
		}
		require.Equal(t, map[string]map[string]int{"app": want, "worker": {"ord": -2}}, got)
	}
}
