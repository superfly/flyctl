package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// createdPlan runs `fly mpg create` without a terminal against mock APIs and
// returns the plan of the cluster it asked for ("" if it asked for none).
func createdPlan(t *testing.T, args ...string) (string, error) {
	t.Helper()

	req, err := createRequest(t, args...)

	return req.Plan, err
}

// createRequest runs `fly mpg create` without a terminal against mock APIs and
// returns the cluster request it sent (empty if it sent none).
func createRequest(t *testing.T, args ...string) (flaps.CreateManagedPostgresClusterRequest, error) {
	t.Helper()

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{})

	flags := newCreate().Flags()
	require.NoError(t, flags.Parse(append([]string{"--name", "db", "--region", "iad"}, args...)))
	ctx = flagctx.NewContext(ctx, flags)

	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{Slug: "personal", RawSlug: "someone", Personal: true}}, nil
		},
	})

	var sent flaps.CreateManagedPostgresClusterRequest
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
		},
		CreateManagedPostgresClusterFunc: func(_ context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			sent = req

			return flaps.ManagedPostgresCluster{}, errors.New("stop after the create request")
		},
	})

	return sent, runCreate(ctx)
}

// --plan names are documented capitalized and matched in any case, but the
// request has to carry the plan's own key: the backend knows Performance as
// "Performance" (#4612), which lowercasing the flag could never match.
func TestRunCreatePlanFlag(t *testing.T) {
	for _, tc := range []struct{ flag, want string }{
		{"performance", "Performance"},
		{"Performance", "Performance"},
		{" PERFORMANCE ", "Performance"},
		{"Basic", "basic"},
		{"launch", "launch"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			plan, _ := createdPlan(t, "--plan", tc.flag)
			assert.Equal(t, tc.want, plan)
		})
	}

	t.Run("unknown plan fails instead of creating a Basic cluster", func(t *testing.T) {
		plan, err := createdPlan(t, "--plan", "hobby")
		require.ErrorContains(t, err, `unknown plan "hobby"`)
		assert.ErrorContains(t, err, "Basic, Starter, Launch, Scale, Performance")
		assert.Empty(t, plan, "no cluster should be requested")
	})

	t.Run("no plan still defaults to Basic without a terminal", func(t *testing.T) {
		plan, _ := createdPlan(t)
		assert.Equal(t, "basic", plan)
	})
}

// Without --volume-size, create sends no disk size (disk_size_gb is left out
// of the request), so the API gives the cluster its plan's default disk:
// 1 GB for dev-256, 10 GB for the others.
func TestRunCreateVolumeSize(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		plan string
		want int
	}{
		{"no --volume-size sends no size", nil, "basic", 0},
		{"--volume-size is sent as given", []string{"--volume-size", "20"}, "basic", 20},
		{"dev-256 without --volume-size sends no size", []string{"--plan", "dev-256"}, "dev-256", 0},
		{"dev-256 with --volume-size sends it", []string{"--plan", "dev-256", "--volume-size", "5"}, "dev-256", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := createRequest(t, tc.args...)
			assert.Equal(t, tc.plan, req.Plan)
			assert.Equal(t, tc.want, req.DiskSizeGB)
		})
	}
}

// dev-256 is hidden until it launches: --plan takes it, in any case, and the
// API decides whether the org can have it, but the plan lists don't show it.
func TestRunCreateHiddenPlan(t *testing.T) {
	for _, flag := range []string{"dev-256", "DEV-256"} {
		plan, _ := createdPlan(t, "--plan", flag)
		assert.Equal(t, "dev-256", plan, flag)
	}

	_, err := createdPlan(t, "--plan", "hobby")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "Dev 256", "the unknown plan error")
	assert.NotContains(t, newFork().Flags().Lookup("plan").Usage, "Dev 256", "fork's --plan help")
}

func TestWarnIfV2FlagUsed(t *testing.T) {
	const want = "The '--v2' flag is deprecated and no longer has any effect."

	cases := []struct {
		name  string
		value string // empty means the flag is left unset
		warn  bool
	}{
		{name: "flag omitted", warn: false},
		{name: "explicit --v2=true", value: "true", warn: true},
		{name: "explicit --v2=false", value: "false", warn: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)

			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flagSet.Bool("v2", true, "")
			if tt.value != "" {
				require.NoError(t, flagSet.Set("v2", tt.value))
			}
			ctx = flagctx.NewContext(ctx, flagSet)

			warnIfV2FlagUsed(ctx)

			if tt.warn {
				assert.Contains(t, stderr.String(), want)
			} else {
				assert.Empty(t, stderr.String())
			}
		})
	}
}
