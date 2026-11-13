package mpg

import (
	"context"
	"errors"
	"slices"
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

// publicPlans is what the plans API lists for an org that isn't offered
// dev-256.
var publicPlans = []flaps.ManagedPostgresPlan{
	{Plan: "basic", CPUKind: "shared", CPUs: 2, MemoryMB: 1024, MonthlyPriceUSD: 38},
	{Plan: "starter", CPUKind: "shared", CPUs: 2, MemoryMB: 2048, MonthlyPriceUSD: 72},
	{Plan: "launch", CPUKind: "performance", CPUs: 2, MemoryMB: 8192, MonthlyPriceUSD: 282},
	{Plan: "scale", CPUKind: "performance", CPUs: 4, MemoryMB: 33792, MonthlyPriceUSD: 962},
	{Plan: "Performance", CPUKind: "performance", CPUs: 8, MemoryMB: 65536, MonthlyPriceUSD: 1922},
}

// dev256 is listed only for orgs that are offered it, and is cheaper than Basic.
var dev256 = flaps.ManagedPostgresPlan{Plan: "dev-256", CPUKind: "shared", CPUs: 1, MemoryMB: 256, MonthlyPriceUSD: 5}

// createRun is what a run of `fly mpg create` did.
type createRun struct {
	plansOrg string // the org it listed plans for
	plan     string // the plan of the cluster it asked for ("" if it asked for none)
	out      string
	err      error
}

// runCreateWith runs `fly mpg create` without a terminal against mock APIs
// whose plans API answers with plans and plansErr. A requested cluster is
// ready at once.
func runCreateWith(t *testing.T, plans []flaps.ManagedPostgresPlan, plansErr error, args ...string) createRun {
	t.Helper()

	ios, _, stdout, _ := iostreams.Test()
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

	var run createRun
	var cluster flaps.ManagedPostgresCluster
	cluster.ID = "mpg-1"
	cluster.Status = flaps.ManagedPostgresStatusReady
	cluster.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresPlansFunc: func(_ context.Context, orgSlug string) ([]flaps.ManagedPostgresPlan, error) {
			run.plansOrg = orgSlug

			return plans, plansErr
		},
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
		},
		CreateManagedPostgresClusterFunc: func(_ context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			run.plan = req.Plan

			return cluster, nil
		},
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return cluster, nil
		},
		GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
			return flaps.ManagedPostgresUserCredentials{Password: "secret"}, nil
		},
	})

	run.err = runCreate(ctx)
	run.out = stdout.String()

	return run
}

// createdPlan runs `fly mpg create` for an org offered the public plans and
// returns the plan of the cluster it asked for ("" if it asked for none).
func createdPlan(t *testing.T, args ...string) (string, error) {
	t.Helper()

	run := runCreateWith(t, publicPlans, nil, args...)

	return run.plan, run.err
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

// dev-256 is offered to an org only when the plans API lists it for that org,
// and flyctl has no details of its own for it, so they come from the API.
func TestRunCreatePlanOnlyTheAPIKnows(t *testing.T) {
	t.Run("selectable when listed for the org", func(t *testing.T) {
		run := runCreateWith(t, append(slices.Clone(publicPlans), dev256), nil, "--plan", "DEV-256")
		require.NoError(t, run.err)
		assert.Equal(t, "someone", run.plansOrg, "plans are listed for the org the cluster goes in")
		assert.Equal(t, "dev-256", run.plan)
		assert.Contains(t, run.out, "Selected Plan: dev-256\n  CPU: Shared x 1\n  Memory: 256 MB\n  Price: $5 per month\n")
	})

	t.Run("unknown when not listed for the org", func(t *testing.T) {
		run := runCreateWith(t, publicPlans, nil, "--plan", "dev-256")
		require.ErrorContains(t, run.err, `unknown plan "dev-256"`)
		assert.Empty(t, run.plan, "no cluster should be requested")
	})
}

func TestRunCreateDefaultsToBasicWithACheaperPlan(t *testing.T) {
	plans := append(slices.Clone(publicPlans), dev256)

	run := runCreateWith(t, plans, nil)
	assert.Equal(t, "basic", run.plan, "without a terminal")

	options, keys, def := planPrompt(orgPlans(flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresPlansFunc: func(context.Context, string) ([]flaps.ManagedPostgresPlan, error) {
			return plans, nil
		},
	}), "someone"))
	assert.Equal(t, "dev-256", keys[0], "the prompt lists plans cheapest first")
	require.Contains(t, options, def)
	assert.Equal(t, "basic", keys[slices.Index(options, def)], "the prompt preselects Basic")
}

// An org that isn't offered Basic gets its cheapest plan by default, with or
// without a terminal, and the summary describes that plan.
func TestRunCreateDefaultsToCheapestWithoutBasic(t *testing.T) {
	plans := []flaps.ManagedPostgresPlan{publicPlans[2], publicPlans[1]}

	run := runCreateWith(t, plans, nil)
	require.NoError(t, run.err)
	assert.Equal(t, "starter", run.plan, "without a terminal")
	assert.Contains(t, run.out, "Selected Plan: Starter\n")
	assert.NotContains(t, run.out, "$0 per month")

	options, keys, def := planPrompt(orgPlans(flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresPlansFunc: func(context.Context, string) ([]flaps.ManagedPostgresPlan, error) {
			return plans, nil
		},
	}), "someone"))
	require.Contains(t, options, def)
	assert.Equal(t, "starter", keys[slices.Index(options, def)], "the prompt preselects the cheapest plan")
}

// The plans API is new; when it fails, create carries on with the plans
// flyctl knows.
func TestRunCreateWhenPlansAPIFails(t *testing.T) {
	plansErr := errors.New("plans API down")

	run := runCreateWith(t, nil, plansErr, "--plan", "launch")
	require.NoError(t, run.err)
	assert.Equal(t, "launch", run.plan)
	assert.Contains(t, run.out, "Selected Plan: Launch\n")

	run = runCreateWith(t, nil, plansErr)
	require.NoError(t, run.err)
	assert.Equal(t, "basic", run.plan)

	run = runCreateWith(t, nil, plansErr, "--plan", "dev-256")
	require.ErrorContains(t, run.err, "valid plans are Basic, Starter, Launch, Scale, Performance")
	assert.Empty(t, run.plan)
}
