package launch

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
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

func TestDetermineOrg(t *testing.T) {
	personalOrg := fly.Organization{
		ID:      "org-id-personal",
		Slug:    "personal",
		RawSlug: "logan-griswold-339",
		Name:    "Logan Griswold",
		Type:    "PERSONAL",
	}
	teamOrg := fly.Organization{
		ID:      "org-id-team",
		Slug:    "my-team",
		RawSlug: "my-team",
		Name:    "My Team",
		Type:    "SHARED",
	}
	orgs := []fly.Organization{personalOrg, teamOrg}

	makeClient := func() *mock.Client {
		return &mock.Client{
			GetOrganizationsFunc: func(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
				return orgs, nil
			},
		}
	}

	t.Run("no org flag defaults to personal", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "logan-griswold-339", org.RawSlug)
	})

	t.Run("canonical personal slug", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "personal")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "logan-griswold-339", org.RawSlug)
	})

	t.Run("real raw slug of personal org", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "logan-griswold-339")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "personal", org.Slug)
		assert.Equal(t, "logan-griswold-339", org.RawSlug)
	})

	t.Run("team org by slug", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "my-team")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "my-team", org.Slug)
	})

	t.Run("org by display name", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "My Team")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "my-team", org.Slug)
	})

	t.Run("unknown org returns error and falls back to personal", func(t *testing.T) {
		ctx := newDetermineOrgCtx(t, "does-not-exist")
		ctx = flyutil.NewContextWithClient(ctx, makeClient())

		org, _, err := determineOrg(ctx, nil)
		assert.Error(t, err)
		require.NotNil(t, org)
		assert.Equal(t, "personal", org.Slug)
	})
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

	t.Run("no flags in non-interactive mode returns error", func(t *testing.T) {
		ctx := newDetermineBaseAppConfigCtx(t, false, false)
		ctx = appconfig.WithConfig(ctx, existingCfg)
		// Non-interactive iostreams → prompt.Confirm returns ErrNonInteractive.
		ios, _, _, _ := iostreams.Test()
		ctx = iostreams.NewContext(ctx, ios)

		_, _, err := determineBaseAppConfig(ctx)
		assert.Error(t, err)
	})
}
