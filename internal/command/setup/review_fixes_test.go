package setup

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/mock"
)

func TestForceReplacesNullServer(t *testing.T) {
	for _, client := range mcpClients {
		if client.format != formatJSON {
			continue
		}
		t.Run(client.slug, func(t *testing.T) {
			original := []byte(fmt.Sprintf(`{"%s":{"fly":null,"other":{"command":"keep"}}}`, client.container))
			out, status, err := mergeJSONBytes(original, client, false)
			require.NoError(t, err)
			require.Equal(t, statusConflict, status)
			require.Nil(t, out)

			out, status, err = mergeJSONBytes(original, client, true)
			require.NoError(t, err)
			require.Equal(t, statusConfigured, status)
			require.Contains(t, string(out), hostedMCPURL)
			require.Contains(t, string(out), `"other":{"command":"keep"}`)
			_, status, err = mergeJSONBytes(out, client, false)
			require.NoError(t, err)
			require.Equal(t, statusAlready, status)
		})
	}
}

func TestRejectedCredentialsOfferLogin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		login  bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "Unauthorized", true},
		{"GraphQL unauthorized", http.StatusOK, `{"errors":[{"message":"Invalid token","extensions":{"code":"UNAUTHORIZED"}}]}`, true},
		{"HTTP forbidden", http.StatusForbidden, "Forbidden", false},
		{"GraphQL unavailable", http.StatusOK, `{"errors":[{"message":"Try again","extensions":{"code":"SERVICE_UNAVAILABLE"}}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			client := fly.NewClientFromOptions(fly.ClientOptions{BaseURL: srv.URL, AccessToken: "expired-token"})
			ctx, _, _, streams, _, _ := harness(t, client)
			streams.SetStdinTTY(true)
			called := false
			loginFn = func(context.Context) error {
				called = true
				return nil
			}

			result := ensureAuth(ctx, streams, false, true, false)
			require.Equal(t, tc.login, called, "%+v", result)
			require.Equal(t, tc.login, result.OK, "%+v", result)
			require.Equal(t, !tc.login, result.Unverified, "%+v", result)
			if tc.login {
				require.Empty(t, result.Error, "do not reuse rejected credentials after login")
			}
		})
	}
}

func TestWrappedAPIUnauthorizedOffersLogin(t *testing.T) {
	checks := 0
	client := &mock.Client{
		AuthenticatedFunc: func() bool { return true },
		GetCurrentUserFunc: func(context.Context) (*fly.User, error) {
			checks++
			return nil, fmt.Errorf("viewer: %w", &fly.ApiError{Status: http.StatusUnauthorized, Message: "401 Unauthorized"})
		},
	}
	ctx, _, _, streams, _, _ := harness(t, client)
	streams.SetStdinTTY(true)
	called := false
	loginFn = func(context.Context) error {
		called = true
		return nil
	}
	result := ensureAuth(ctx, streams, false, true, false)
	require.True(t, called)
	require.True(t, result.OK)
	require.Empty(t, result.Error)
	require.Equal(t, 1, checks, "do not reuse the old client after saving a new token")
}

func TestAncestorSymlinkOutsideHomeIsRefused(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
			outside, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			config := filepath.Join(outside, "config", "mcp_config.json")
			original := []byte(`{"keep":true}`)
			if existing {
				writeExisting(t, config, original, 0o600)
			}
			if err := os.Symlink(outside, filepath.Join(home, ".gemini")); err != nil {
				t.Skip(err)
			}

			result := configureMCP(home, "antigravity", true, false)
			require.Equal(t, statusFailed, result.Status, result.Detail)
			require.Contains(t, result.Detail, "outside the expected config directory")
			if existing {
				require.Equal(t, original, readFile(t, config))
			} else {
				require.NoDirExists(t, filepath.Dir(config), "must not create directories outside home")
			}
		})
	}
}

func TestSwappedResolvedParentCannotRedirectWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps opened directories from being renamed")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	realDir := filepath.Join(home, "real-config")
	target := filepath.Join(realDir, "authorized_keys")
	writeExisting(t, target, []byte(`{"keep":true}`), 0o640)
	ssh := filepath.Join(home, ".ssh")
	keys := filepath.Join(ssh, "authorized_keys")
	original := []byte("ssh-ed25519 AAAA keep\n")
	writeExisting(t, keys, original, 0o600)
	require.NoError(t, os.Symlink(target, filepath.Join(home, ".claude.json")))
	beforeWriteHook = func() {
		require.NoError(t, os.Rename(realDir, realDir+"-old"))
		require.NoError(t, os.Symlink(ssh, realDir))
	}
	t.Cleanup(func() { beforeWriteHook = nil })

	result := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, result.Status, result.Detail)
	require.Equal(t, original, readFile(t, keys))
	require.Contains(t, string(readFile(t, filepath.Join(realDir+"-old", "authorized_keys"))), hostedMCPURL)
	entries, err := os.ReadDir(ssh)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary files may be created in the substituted directory")
}

func TestConfigDirectoryWalkRejectsSubstitutedSymlink(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, "config")); err != nil {
		t.Skip(err)
	}
	// The path passed to openConfigDir was already canonicalized; any
	// symlink encountered now must have appeared since that check.
	dir, err := openConfigDir(filepath.Join(home, "config", "missing"))
	require.Error(t, err)
	require.Nil(t, dir)
	require.NoDirExists(t, filepath.Join(outside, "missing"))
}

func TestReadonlyConfigRetainsPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	path := filepath.Join(home, ".claude.json")
	writeExisting(t, path, []byte(`{}`), 0o444)
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	result := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, result.Status, result.Detail)
	require.Contains(t, string(readFile(t, path)), hostedMCPURL)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o444), info.Mode().Perm())
}
