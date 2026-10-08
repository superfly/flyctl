package launch

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newDetermineOrgCtx(t *testing.T, orgFlag string) context.Context {
	t.Helper()

	ctx := context.Background()

	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("org", "", "")
	flagSet.Bool("attach", false, "")

	if orgFlag != "" {
		require.NoError(t, flagSet.Set("org", orgFlag))
	}

	return flagctx.NewContext(ctx, flagSet)
}

func TestAppNameTaken(t *testing.T) {
	someErr := errors.New("flaps is having a day")

	for _, tc := range []struct {
		name      string
		available bool
		err       error
		wantTaken bool
		wantErr   error
	}{
		{name: "available", available: true},
		{name: "taken", available: false, wantTaken: true},
		{name: "error propagates", err: someErr, wantErr: someErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked string
			ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
				AppNameAvailableFunc: func(ctx context.Context, name string) (bool, error) {
					asked = name

					return tc.available, tc.err
				},
			})

			taken, err := appNameTaken(ctx, "some-app")

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantTaken, taken)
			assert.Equal(t, "some-app", asked)
		})
	}
}

func TestDetermineOrg(t *testing.T) {
	personalOrg := uiex.Organization{
		ID:       "org-id-personal",
		Slug:     "personal",
		RawSlug:  "lubien-org-339",
		Name:     "Lubien Org",
		Personal: true,
	}
	teamOrg := uiex.Organization{
		ID:      "org-id-team",
		Slug:    "my-team",
		RawSlug: "my-team",
		Name:    "My Team",
	}
	orgs := []uiex.Organization{personalOrg, teamOrg}

	makeClient := func() *mock.UiexClient {
		return &mock.UiexClient{
			ListOrganizationsFunc: func(ctx context.Context, admin bool) ([]uiex.Organization, error) {
				return orgs, nil
			},
		}
	}

	t.Run("no org flag defaults to personal", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "lubien-org-339", org.RawSlug)
	})

	t.Run("canonical personal slug", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "personal")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "lubien-org-339", org.RawSlug)
	})

	t.Run("real raw slug of personal org", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "lubien-org-339")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "lubien-org-339", org.RawSlug)
	})

	t.Run("team org by slug", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "my-team")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "my-team", org.Slug)
	})

	t.Run("org by display name", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "My Team")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "my-team", org.Slug)
	})

	t.Run("unknown org returns error and falls back to personal", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "does-not-exist")
		ctx = uiexutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		assert.Error(t, err)
		require.NotNil(t, org)
		assert.Equal(t, "personal", org.Slug)
	})
}

// newBuildManifestCtx is a non-interactive context carrying every launch flag,
// with --image set so no source scanning happens.
func newBuildManifestCtx(t *testing.T) context.Context {
	t.Helper()

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)

	flags := New().Flags()
	require.NoError(t, flags.Set("image", "nginx:alpine"))
	require.NoError(t, flags.Set("path", t.TempDir()))

	return flagctx.NewContext(ctx, flags)
}

// An organization with no payment method has a machine limit of zero, so the
// placements request that picks the region fails with "requested machine
// count exceeds organization limit", which never mentions billing. The billing
// check has to run before it.
func TestBuildManifestChecksBillingBeforePlacement(t *testing.T) {
	errOrgLimit := errors.New("requested machine count exceeds organization limit")

	for _, tc := range []struct {
		name       string
		status     uiex.BillingStatus
		planStep   string
		wantPlaced bool
	}{
		{name: "no payment method", status: uiex.BillingStatusSourceRequired},
		{name: "trial ended", status: uiex.BillingStatusTrialEnded},
		{name: "trial active", status: uiex.BillingStatusTrialActive, wantPlaced: true},
		{name: "current", status: uiex.BillingStatusCurrent, wantPlaced: true},
		{name: "past due", status: uiex.BillingStatusPastDue, wantPlaced: true},
		{name: "unknown status", status: "SOMETHING_NEW", wantPlaced: true},
		// The deployer drives launch through plan steps; it never had this check.
		{name: "plan step", status: uiex.BillingStatusSourceRequired, planStep: "propose", wantPlaced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newBuildManifestCtx(t)
			if tc.planStep != "" {
				ctx = context.WithValue(ctx, plan.PlanStepKey, tc.planStep)
			}
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
					return []uiex.Organization{{Slug: "personal", RawSlug: "someone-123", Personal: true, BillingStatus: tc.status}}, nil
				},
			})

			placed := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					return nil, errors.New("app not found")
				},
				AppNameAvailableFunc: func(context.Context, string) (bool, error) { return true, nil },
				// Stop the build right here either way; only the ordering matters.
				GetPlacementsFunc: func(context.Context, *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error) {
					placed = true

					return nil, errOrgLimit
				},
			})

			_, _, err := buildManifest(ctx, nil, &recoverableErrorBuilder{canEnterUi: false})
			require.Error(t, err)
			assert.Equal(t, tc.wantPlaced, placed, "placements requested")

			if !tc.wantPlaced {
				assert.Contains(t, err.Error(), "payment method")
				assert.Contains(t, flyerr.GetErrorSuggestion(err), "https://fly.io/dashboard/personal/billing")
			}
		})
	}
}

// A manifest given with --from-manifest skips buildManifest, so its billing
// check has to happen when the launch state is built from the manifest. A
// manifest buildManifest just produced was checked already, and plan steps (the
// deployer) skip the check.
func TestStateFromManifestChecksBilling(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cache    planBuildCache
		planStep string
		wantErr  bool
	}{
		{name: "manifest loaded from a file", cache: planBuildCache{appNameValidated: true}, wantErr: true},
		{name: "manifest from buildManifest", cache: planBuildCache{appNameValidated: true, billingChecked: true}},
		{name: "plan step", cache: planBuildCache{appNameValidated: true}, planStep: "create"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newBuildManifestCtx(t)
			require.NoError(t, flag.FromContext(ctx).Set("no-deploy", "true")) // no remote builder warm-up
			if tc.planStep != "" {
				ctx = context.WithValue(ctx, plan.PlanStepKey, tc.planStep)
			}
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
					return &uiex.Organization{Slug: "personal", BillingStatus: uiex.BillingStatusSourceRequired}, nil
				},
			})

			m := LaunchManifest{
				Plan:       &plan.LaunchPlan{AppName: "my-app", OrgSlug: "personal"},
				PlanSource: newDefaultPlanSource("from manifest"),
				Config:     appconfig.NewConfig(),
			}
			tc.cache.appConfig = m.Config

			_, err := stateFromManifest(ctx, m, &tc.cache, &recoverableErrorBuilder{canEnterUi: false})

			if !tc.wantErr {
				require.NoError(t, err)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "payment method")
			assert.Contains(t, flyerr.GetErrorSuggestion(err), "https://fly.io/dashboard/personal/billing")
		})
	}
}

// newDetermineBaseAppConfigCtx builds a context wired with the flags that
// determineBaseAppConfig reads. Pass configPath="" to leave --config unset.
func newDetermineBaseAppConfigCtx(t *testing.T, copyConfigFlag, explicitConfigPath bool) context.Context {
	t.Helper()

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, iostreams.System())

	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("config", "", "")
	flagSet.Bool("copy-config", false, "")
	flagSet.Bool("attach", false, "")
	flagSet.Bool("yes", false, "")

	if copyConfigFlag {
		require.NoError(t, flagSet.Set("copy-config", "true"))
	}
	if explicitConfigPath {
		require.NoError(t, flagSet.Set("config", "fly.custom.toml"))
	}

	return flagctx.NewContext(ctx, flagSet)
}

func TestDetermineBaseAppConfig(t *testing.T) {
	// existingCfg simulates what LoadAppConfigIfPresent puts in context when
	// the customer has a custom fly.toml with a non-default dockerfile.
	existingCfg := appconfig.NewConfig()
	existingCfg.Build = &appconfig.Build{
		Dockerfile: "docker.ui-server.dockerfile",
	}

	t.Run("no flags and no existing config returns blank config", func(t *testing.T) {
		ctx := newDetermineBaseAppConfigCtx(t, false, false)
		// No config in context — simulates no fly.toml present.

		cfg, copied, err := determineBaseAppConfig(ctx)
		require.NoError(t, err)
		assert.False(t, copied)
		assert.Nil(t, cfg.Build)
	})

	t.Run("--copy-config adopts existing config without prompting", func(t *testing.T) {
		ctx := newDetermineBaseAppConfigCtx(t, true, false)
		ctx = appconfig.WithConfig(ctx, existingCfg)

		cfg, copied, err := determineBaseAppConfig(ctx)
		require.NoError(t, err)
		assert.True(t, copied)
		assert.Equal(t, "docker.ui-server.dockerfile", cfg.Build.Dockerfile)
	})

	t.Run("explicit --config adopts existing config without prompting", func(t *testing.T) {
		// This is the deployer scenario: --config fly.custom.toml is passed but
		// --copy-config is not. The explicit path signals intent, so we must
		// not fall through to source scanning with an empty config.
		ctx := newDetermineBaseAppConfigCtx(t, false, true)
		ctx = appconfig.WithConfig(ctx, existingCfg)

		cfg, copied, err := determineBaseAppConfig(ctx)
		require.NoError(t, err)
		assert.True(t, copied)
		assert.Equal(t, "docker.ui-server.dockerfile", cfg.Build.Dockerfile)
	})

	t.Run("no flags in non-interactive mode returns error naming the flags", func(t *testing.T) {
		ctx := newDetermineBaseAppConfigCtx(t, false, false)
		ctx = appconfig.WithConfig(ctx, existingCfg)
		// Non-interactive iostreams → prompt.Confirm returns ErrNonInteractive.
		ios, _, _, _ := iostreams.Test()
		ctx = iostreams.NewContext(ctx, ios)

		_, _, err := determineBaseAppConfig(ctx)
		require.ErrorIs(t, err, prompt.ErrNonInteractive)
		assert.Contains(t, err.Error(), "--copy-config")
		assert.Contains(t, err.Error(), "fly deploy")
	})
}

// newNudgeCtx is a non-interactive context carrying every launch flag, where
// the user can see an app named "existing-app".
func newNudgeCtx(t *testing.T, args ...string) context.Context {
	t.Helper()

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)

	flags := New().Flags()
	require.NoError(t, flags.Parse(args))
	ctx = flagctx.NewContext(ctx, flags)

	return flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
			if name == "existing-app" {
				return &flaps.App{Name: name}, nil
			}

			return nil, errors.New("app not found")
		},
	})
}

// Launching under the name of an app the user already has usually means they
// wanted `fly deploy`. Interactively they are asked; headless there is nobody
// to ask, and a new app under a generated name is the one outcome nobody chose.
func TestNudgeTowardsDeployNonInteractive(t *testing.T) {
	t.Run("existing app fails with directions", func(t *testing.T) {
		taken, err := nudgeTowardsDeploy(newNudgeCtx(t), "existing-app")

		require.ErrorIs(t, err, prompt.ErrNonInteractive)
		assert.True(t, taken)
		assert.Contains(t, err.Error(), "fly deploy")
		assert.Contains(t, err.Error(), "--generate-name")
	})

	t.Run("unknown app proceeds", func(t *testing.T) {
		taken, err := nudgeTowardsDeploy(newNudgeCtx(t), "new-app")

		require.NoError(t, err)
		assert.False(t, taken)
	})

	t.Run("--yes launches a new app", func(t *testing.T) {
		taken, err := nudgeTowardsDeploy(newNudgeCtx(t, "--yes"), "existing-app")

		require.NoError(t, err)
		assert.False(t, taken)
	})

	t.Run("plan steps keep generating a name", func(t *testing.T) {
		// The deployer drives launch through plan steps, headless.
		ctx := context.WithValue(newNudgeCtx(t), plan.PlanStepKey, "propose")

		taken, err := nudgeTowardsDeploy(ctx, "existing-app")

		require.NoError(t, err)
		assert.True(t, taken)
	})
}
