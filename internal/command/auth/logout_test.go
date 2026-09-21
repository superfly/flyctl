package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// logoutTestDir returns a scratch config directory and points both the config
// lock file and the agent socket at it, so tests neither touch the source tree
// nor talk to an agent the developer happens to be running.
func logoutTestDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("FLY_CONFIG_DIR", dir)

	return dir
}

// logoutTestContext builds the context runLogout expects, and returns the
// buffers backing its output streams.
func logoutTestContext(t *testing.T, dir, apiBaseURL string, force bool) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	fs := pflag.NewFlagSet("logout", pflag.ContinueOnError)
	fs.Bool("force", false, "")
	if force {
		require.NoError(t, fs.Set("force", "true"))
	}

	streams, _, stdout, stderr := iostreams.Test()
	ctx := flag.NewContext(t.Context(), fs)
	// Mirror config.Load, which always leaves Tokens non-nil.
	ctx = config.NewContext(ctx, &config.Config{
		APIBaseURL: apiBaseURL,
		Tokens:     new(tokens.Tokens),
	})
	ctx = state.WithConfigDirectory(ctx, dir)
	ctx = iostreams.NewContext(ctx, streams)

	return ctx, stdout, stderr
}

func logoutTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server
}

func TestRevokeSavedTokenUsesConfigToken(t *testing.T) {
	const savedToken = "saved-token"
	dir := logoutTestDir(t)
	t.Setenv(config.APITokenEnvKey, "environment-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+savedToken, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"logOut":{"ok":true}}}`))
	}))
	defer server.Close()

	path := filepath.Join(dir, config.FileName)
	require.NoError(t, config.SetAccessToken(path, savedToken))
	ctx := config.NewContext(context.Background(), &config.Config{APIBaseURL: server.URL})

	revoked, err := revokeSavedToken(ctx, path)
	require.NoError(t, err)
	require.True(t, revoked)
}

// A revoked token must never be left on disk: revoking it again would keep
// failing, so every later logout would fail too.
func TestRunLogoutClearsSavedTokenOnSuccess(t *testing.T) {
	dir := logoutTestDir(t)
	t.Setenv(config.AccessTokenEnvKey, "")
	t.Setenv(config.APITokenEnvKey, "")

	path := filepath.Join(dir, config.FileName)
	require.NoError(t, config.SetAccessToken(path, "saved-token"))

	server := logoutTestServer(t, `{"data":{"logOut":{"ok":true}}}`)
	ctx, stdout, _ := logoutTestContext(t, dir, server.URL, false)

	require.NoError(t, runLogout(ctx))
	require.Contains(t, stdout.String(), "logged out successfully")

	token, err := config.ReadAccessToken(path)
	require.NoError(t, err)
	require.Empty(t, token)
}

func TestRunLogoutKeepsSavedTokenWhenRevokeFails(t *testing.T) {
	const savedToken = "saved-token"
	dir := logoutTestDir(t)
	t.Setenv(config.AccessTokenEnvKey, "")
	t.Setenv(config.APITokenEnvKey, "")

	path := filepath.Join(dir, config.FileName)
	require.NoError(t, config.SetAccessToken(path, savedToken))

	server := logoutTestServer(t, `{"data":{"logOut":{"ok":false}}}`)
	ctx, stdout, _ := logoutTestContext(t, dir, server.URL, false)

	err := runLogout(ctx)
	require.ErrorContains(t, err, "failed to revoke saved token")
	// The user needs a way out of a revoke that keeps failing.
	require.ErrorContains(t, err, "fly auth logout --force")
	require.Empty(t, stdout.String())

	token, readErr := config.ReadAccessToken(path)
	require.NoError(t, readErr)
	require.Equal(t, savedToken, token)
}

// --force is the escape hatch for a token that can never be revoked again,
// such as one that was already revoked or has expired.
func TestRunLogoutForceClearsSavedTokenWhenRevokeFails(t *testing.T) {
	dir := logoutTestDir(t)
	t.Setenv(config.AccessTokenEnvKey, "")
	t.Setenv(config.APITokenEnvKey, "")

	path := filepath.Join(dir, config.FileName)
	require.NoError(t, config.SetAccessToken(path, "saved-token"))

	server := logoutTestServer(t, `{"data":{"logOut":{"ok":false}}}`)
	ctx, stdout, stderr := logoutTestContext(t, dir, server.URL, true)

	require.NoError(t, runLogout(ctx))
	require.Contains(t, stdout.String(), "removed local access token without revoking it")
	require.Contains(t, stderr.String(), "remains valid")

	token, err := config.ReadAccessToken(path)
	require.NoError(t, err)
	require.Empty(t, token)
	require.NotContains(t, stdout.String()+stderr.String(), "saved-token")
}

func TestLogoutTokenOverrideWarning(t *testing.T) {
	t.Run("FLY_API_TOKEN", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "restored-after-test")
		require.NoError(t, os.Unsetenv(config.AccessTokenEnvKey))
		t.Setenv(config.APITokenEnvKey, "api-token")

		warning := logoutTokenOverrideWarning()
		require.Contains(t, warning, config.APITokenEnvKey)
		require.Contains(t, warning, "not revoked or removed")
	})

	t.Run("both token environment variables", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "access-token")
		t.Setenv(config.APITokenEnvKey, "api-token")

		warning := logoutTokenOverrideWarning()
		require.Contains(t, warning, config.AccessTokenEnvKey)
		require.Contains(t, warning, config.APITokenEnvKey)
		require.Contains(t, warning, "continue using them")
		require.Contains(t, warning, "Unset both variables")
	})

	// config.applyEnv takes the first *present* variable, so an explicitly empty
	// FLY_ACCESS_TOKEN keeps FLY_API_TOKEN from being used at all.
	t.Run("empty FLY_ACCESS_TOKEN blocks FLY_API_TOKEN", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "")
		t.Setenv(config.APITokenEnvKey, "api-token")

		require.Empty(t, logoutTokenOverrideWarning())
	})

	t.Run("no active environment token", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "")
		t.Setenv(config.APITokenEnvKey, "")

		require.Empty(t, logoutTokenOverrideWarning())
	})
}
