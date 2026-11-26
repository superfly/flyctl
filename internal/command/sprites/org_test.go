package sprites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/spritestate"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

// withOrgs gives ctx a Fly.io API that knows these organizations.
func withOrgs(ctx context.Context, orgs ...uiex.Organization) context.Context {
	return uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return orgs, nil
		},
		GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
			for _, org := range orgs {
				if org.Slug == slug {
					return &org, nil
				}
			}

			return nil, fmt.Errorf("organization %s not found", slug)
		},
	})
}

func TestCanonicalOrgResolvesPersonal(t *testing.T) {
	e := newEnv(t, newList())
	ctx := withOrgs(e.ctx, uiex.Organization{Slug: "personal", RawSlug: "alex-123", Personal: true})

	// The Sprites API and the .sprite file use the real slug, not the alias.
	org, err := canonicalOrg(ctx, "personal")
	require.NoError(t, err)
	assert.Equal(t, "alex-123", org)

	// An organization the API does not report a raw slug for keeps its own.
	ctx = withOrgs(e.ctx, uiex.Organization{Slug: "personal"})
	org, err = canonicalOrg(ctx, "personal")
	require.NoError(t, err)
	assert.Equal(t, "personal", org)

	_, err = canonicalOrg(withOrgs(e.ctx), "personal")
	require.ErrorContains(t, err, "not found")
}

func TestResolveOrgFromFlagResolvesPersonal(t *testing.T) {
	e := newEnv(t, newList(), "--org", "personal")
	ctx := withOrgs(e.ctx, uiex.Organization{Slug: "personal", RawSlug: "alex-123", Personal: true})

	org, err := resolveOrg(ctx)
	require.NoError(t, err)
	assert.Equal(t, "alex-123", org)
}

func TestResolveOrgSelectsOnlyOrgAndRemembersIt(t *testing.T) {
	e := newEnv(t, newList())
	ctx := withOrgs(e.ctx, uiex.Organization{Slug: "personal", RawSlug: "alex-123", Name: "Alex", Personal: true})

	// With one organization there is nothing to ask.
	org, err := resolveOrg(ctx)
	require.NoError(t, err)
	assert.Equal(t, "alex-123", org)

	// It is saved, so later commands do not look it up or prompt again.
	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, "alex-123", st.DefaultOrg)

	org, err = resolveOrg(withOrgs(e.ctx))
	require.NoError(t, err)
	assert.Equal(t, "alex-123", org)
}

func TestResolveOrgNeedsOrgWhenItCannotAsk(t *testing.T) {
	e := newEnv(t, newList())
	ctx := withOrgs(e.ctx,
		uiex.Organization{Slug: "personal", RawSlug: "alex-123", Personal: true},
		uiex.Organization{Slug: "acme", RawSlug: "acme"},
	)

	// Several organizations and no terminal to choose in.
	_, err := resolveOrg(ctx)
	require.ErrorContains(t, err, "no organization specified")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "--org")

	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Empty(t, st.DefaultOrg, "no default is saved when none was chosen")
}

func TestResolveOrgReportsBrokenSpriteFile(t *testing.T) {
	e := newEnv(t, newList())
	writeFile(t, e.workDir, ".sprite", "{not json")

	_, err := resolveOrg(e.ctx)
	require.ErrorContains(t, err, "failed to parse")

	_, err = (&session{Org: "acme"}).namedSprite(e.ctx, "")
	require.ErrorContains(t, err, "failed to parse")
}

func TestNewSessionMintsForResolvedOrg(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newList(), "--org", "acme")
	t.Setenv("SPRITES_API_URL", api.URL+"/")

	original := createToken
	t.Cleanup(func() { createToken = original })

	createToken = func(_ context.Context, _, org, apiURL string) (string, error) {
		assert.Equal(t, "acme", org)
		assert.Equal(t, api.URL, apiURL, "a trailing slash on the API URL is dropped")

		return "test-token", nil
	}

	s, err := newSession(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "acme", s.Org)
	assert.Equal(t, "acme", s.orgDisplay())
	assert.False(t, s.cached)

	// The second session finds the saved token.
	s, err = newSession(e.ctx)
	require.NoError(t, err)
	assert.True(t, s.cached)
}

func TestNewSessionReportsMintFailure(t *testing.T) {
	e := newEnv(t, newList(), "--org", "acme")

	original := createToken
	t.Cleanup(func() { createToken = original })

	createToken = func(context.Context, string, string, string) (string, error) {
		return "", errors.New("not a member")
	}

	_, err := newSession(e.ctx)
	require.ErrorContains(t, err, "failed to get a Sprites token for organization acme: not a member")

	// And so does a command run through withSession, without running.
	ran := false
	err = withSession(func(context.Context, *session) error {
		ran = true
		return nil
	})(e.ctx)

	require.Error(t, err)
	assert.False(t, ran)
}

func TestExplainLimitForCreationRate(t *testing.T) {
	err := explainLimit(&sprites.APIError{
		StatusCode:        http.StatusTooManyRequests,
		ErrorCode:         sprites.ErrCodeCreationRateLimited,
		Message:           "slow down",
		Limit:             10,
		WindowSeconds:     60,
		RetryAfterSeconds: 30,
		UpgradeURL:        "https://sprites.dev/upgrade",
	})

	advice := flyerr.GetErrorSuggestion(err)
	assert.Contains(t, advice, "10 sprites per 60s")
	assert.Contains(t, advice, "Try again in 30s")
	assert.Contains(t, advice, "https://sprites.dev/upgrade")

	assert.NoError(t, explainLimit(nil))
}

func TestIsLetter(t *testing.T) {
	for _, b := range []byte("aZcQ") {
		assert.True(t, isLetter(b))
	}

	for _, b := range []byte("1:/ ") {
		assert.False(t, isLetter(b))
	}
}

func TestCompileTerminfo(t *testing.T) {
	for _, tool := range []string{"infocmp", "tic"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}

	// A terminal described only through infocmp is compiled into an entry.
	data := compileTerminfo(context.Background(), "xterm", []string{"x", "78"})
	assert.NotEmpty(t, data)

	assert.Nil(t, compileTerminfo(context.Background(), "no-such-terminal-zzz", []string{"n", "6e"}))
}
