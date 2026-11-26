package mpg

import (
	"context"
	"errors"
	"strings"
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
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
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

	// An organization that can use Basic, as every organization could before the Developer plan.
	req, _, err := createRequestWithPlans(t, []flaps.ManagedPostgresPlan{{Plan: "dev-256"}, {Plan: "basic"}}, nil, false, args...)

	return req, err
}

// createRequestWithPlans is createRequest for an organization whose plan
// lookup returns plans and plansErr. It also returns what was written to stderr.
func createRequestWithPlans(t *testing.T, plans []flaps.ManagedPostgresPlan, plansErr error, interactive bool, args ...string) (flaps.CreateManagedPostgresClusterRequest, string, error) {
	t.Helper()

	ios, _, _, stderr := iostreams.Test()
	ios.SetStdinTTY(interactive)
	ios.SetStdoutTTY(interactive)
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{})

	flags := newCreate().Flags()
	require.NoError(t, flags.Parse(append([]string{"--name", "db", "--region", "iad"}, args...)))
	ctx = flagctx.NewContext(ctx, flags)

	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{Slug: "personal", RawSlug: "someone", Personal: true, InternalNumericID: 7}}, nil
		},
	})

	checkOrg := authtest.Organization(t, 7)
	var sent flaps.CreateManagedPostgresClusterRequest
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresPlansFunc: func(ctx context.Context, slug string) ([]flaps.ManagedPostgresPlan, error) {
			checkOrg(ctx)
			assert.Equal(t, "someone", slug, "the lookup needs the real slug, not personal")

			return plans, plansErr
		},
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
		},
		CreateManagedPostgresClusterFunc: func(ctx context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			checkOrg(ctx)
			sent = req

			return flaps.ManagedPostgresCluster{}, errors.New("stop after the create request")
		},
	})

	err := runCreate(ctx)

	return sent, stderr.String(), err
}

func TestRunCreatePGMajorVersion(t *testing.T) {
	t.Run("no flag asks for the default", func(t *testing.T) {
		req, _ := createRequest(t)
		assert.Equal(t, "17", req.PGMajorVersion)
	})

	for _, version := range []string{"16", "17", "18"} {
		t.Run("accepts "+version, func(t *testing.T) {
			req, _ := createRequest(t, "--pg-major-version", version)
			assert.Equal(t, version, req.PGMajorVersion)
		})
	}

	for _, version := range []string{"15", "19", "0"} {
		t.Run("rejects "+version, func(t *testing.T) {
			req, err := createRequest(t, "--pg-major-version", version)
			assert.EqualError(t, err, "invalid Postgres major version: "+version+". Supported versions are 16, 17 and 18")
			assert.Empty(t, req, "no cluster should be requested")
		})
	}
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
		{"dev-256", "dev-256"},
		{"DEV-256", "dev-256"},
		{"Developer", "dev-256"},
		{"developer", "dev-256"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			plan, _ := createdPlan(t, "--plan", tc.flag)
			assert.Equal(t, tc.want, plan)
		})
	}

	t.Run("unknown plan fails instead of creating a Basic cluster", func(t *testing.T) {
		plan, err := createdPlan(t, "--plan", "hobby")
		require.ErrorContains(t, err, `unknown plan "hobby"`)
		assert.ErrorContains(t, err, "Dev-256, Basic, Starter, Launch, Scale, Performance")
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

// Without --plan, create asks which plans the organization can use: Basic if
// it is one of them, otherwise the cheapest. A Free organization is offered
// only Developer (dev-256), so it must not be sent Basic.
func TestRunCreateDefaultPlan(t *testing.T) {
	const warning = "Warning: lookup failed. Defaulting to Basic; pass --plan to choose.\n"
	free := []flaps.ManagedPostgresPlan{{Plan: "dev-256"}}
	paid := []flaps.ManagedPostgresPlan{{Plan: "starter"}, {Plan: "basic"}, {Plan: "dev-256"}}

	for _, tc := range []struct {
		name     string
		plans    []flaps.ManagedPostgresPlan
		plansErr error
		args     []string
		want     string
		wantErr  string
		warns    bool
	}{
		{name: "Free organization gets Developer", plans: free, want: "dev-256"},
		{name: "paid organization keeps Basic", plans: paid, want: "basic"},
		{name: "no plans is an error", wantErr: "no Managed Postgres plans available for organization someone"},
		{name: "no usable plan is an error", plans: []flaps.ManagedPostgresPlan{{Plan: "future"}}, wantErr: "no Managed Postgres plans available for organization someone"},
		{name: "failed lookup warns and uses Basic", plansErr: errors.New("lookup failed"), want: "basic", warns: true},
		{name: "--plan skips the lookup", plansErr: errors.New("lookup failed"), args: []string{"--plan", "developer"}, want: "dev-256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, stderr, err := createRequestWithPlans(t, tc.plans, tc.plansErr, false, tc.args...)
			if tc.wantErr == "" {
				tc.wantErr = "stop after the create request"
			}
			require.ErrorContains(t, err, tc.wantErr)
			if tc.want == "" {
				assert.EqualError(t, err, tc.wantErr)
				assert.NotContains(t, err.Error(), "--plan")
			}
			assert.Equal(t, tc.want, req.Plan)
			assert.Equal(t, tc.warns, strings.Contains(stderr, warning), "stderr: %s", stderr)
		})
	}
}

func TestRunCreateInteractivePlanLookupFailure(t *testing.T) {
	req, stderr, err := createRequestWithPlans(t, nil, errors.New("lookup failed"), true)
	// The buffer streams cannot render Survey's prompt, so reaching Select
	// returns ErrNonInteractive even though IsInteractive is true.
	require.ErrorIs(t, err, prompt.ErrNonInteractive)
	assert.Empty(t, req)
	assert.NotContains(t, stderr, "Warning:")
	assert.NotContains(t, stderr, "Defaulting to Basic")
}

// With a terminal, the prompt lists only the organization's plans, cheapest
// first by flyctl's own prices, and preselects the plan used without one.
func TestPlanPromptOptions(t *testing.T) {
	const developer, basic = "Developer: Shared x 1, 256 MB RAM, $4/mo", "Basic: Shared x 2, 1 GB RAM, $38/mo"

	for _, tc := range []struct {
		name        string
		plans       []string
		wantOptions []string
		wantKeys    []string
		wantDefault string
	}{
		{"Free organization", []string{"dev-256"}, []string{developer}, []string{"dev-256"}, developer},
		{
			"paid organization", []string{"Performance", "future", "basic", "dev-256"},
			[]string{developer, basic, "Performance: Performance x 8, 64 GB RAM, $1922/mo"},
			[]string{"dev-256", "basic", "Performance"}, basic,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
				ListManagedPostgresPlansFunc: func(context.Context, string) (plans []flaps.ManagedPostgresPlan, err error) {
					for _, key := range tc.plans {
						plans = append(plans, flaps.ManagedPostgresPlan{Plan: key})
					}

					return plans, nil
				},
			})
			plans, err := availablePlans(ctx, "org")
			require.NoError(t, err)

			options, keys, defaultOption := planPromptOptions(plans, defaultPlan(plans))
			assert.Equal(t, tc.wantOptions, options)
			assert.Equal(t, tc.wantKeys, keys)
			assert.Equal(t, tc.wantDefault, defaultOption)
		})
	}
}
