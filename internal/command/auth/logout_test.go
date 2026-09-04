package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/config"
)

func TestRevokeSavedTokenUsesConfigToken(t *testing.T) {
	const savedToken = "saved-token"
	t.Cleanup(func() { _ = os.Remove("flyctl.config.lock") })
	t.Setenv(config.APITokenEnvKey, "environment-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+savedToken, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"logOut":{"ok":true}}}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	require.NoError(t, config.SetAccessToken(path, savedToken))
	ctx := config.NewContext(context.Background(), &config.Config{APIBaseURL: server.URL})

	revoked, err := revokeSavedToken(ctx, path)
	require.NoError(t, err)
	require.True(t, revoked)
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

	t.Run("no active environment token", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "")
		t.Setenv(config.APITokenEnvKey, "")

		require.Empty(t, logoutTokenOverrideWarning())
	})
}
