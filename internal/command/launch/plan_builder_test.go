package launch

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
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
