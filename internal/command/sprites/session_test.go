package sprites

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/spritestate"
	"github.com/superfly/flyctl/internal/state"
)

func TestFilePullRefreshesRejectedSavedToken(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/tmp/file", "contents", "0644")
	dest := filepath.Join(t.TempDir(), "file")
	e := newEnv(t, newFilePull(), "--org", "acme", "--sprite", "dev", "/tmp/file", dest)
	t.Setenv("SPRITES_API_URL", api.URL)
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{spritestate.TokenKey(api.URL, "acme"): "revoked"},
	}))

	original := createToken
	t.Cleanup(func() { createToken = original })
	minted := 0
	createToken = func(context.Context, string, string, string) (string, error) {
		minted++
		return "test-token", nil
	}

	// The fake API returns a structured 401 whose "error" field contains no
	// status code. The SDK alone turns that into "stat /tmp/file: unauthorized".
	require.NoError(t, withSession(runFilePull)(e.ctx))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "contents", string(data))
	require.NoError(t, withSession(runFilePull)(e.ctx))
	assert.Equal(t, 1, minted)
}

func TestResolveTokenRejectsPreviousLogin(t *testing.T) {
	e := newEnv(t, newList(), "--org", "acme")
	require.NoError(t, config.SetAccessToken(state.ConfigFile(e.ctx), "fm2_new-login"))
	key := spritestate.TokenKey(defaultAPIURL, "acme")
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{key: "new-login-sprite-token"},
	}))

	original := createToken
	t.Cleanup(func() { createToken = original })
	createToken = func(context.Context, string, string, string) (string, error) {
		t.Fatal("a command prepared under the previous login must not mint a token")
		return "", nil
	}

	_, _, err := resolveToken(e.ctx, defaultAPIURL, "acme")
	require.ErrorContains(t, err, "login changed")
	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, "new-login-sprite-token", st.Tokens[key])
}

func TestResolveTokenKeepsCacheAfterDischargeRefresh(t *testing.T) {
	e := newEnv(t, newList())
	config.FromContext(e.ctx).Tokens = tokens.ParseFromFile("fo1_login,fm2_old-discharge", state.ConfigFile(e.ctx))
	require.NoError(t, config.SetAccessToken(state.ConfigFile(e.ctx), "fo1_login,fm2_new-discharge"))
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{spritestate.TokenKey(defaultAPIURL, "acme"): "cached"},
	}))

	token, cached, err := resolveToken(e.ctx, defaultAPIURL, "acme")
	require.NoError(t, err)
	assert.Equal(t, "cached", token)
	assert.True(t, cached)
}

func TestForgetTokenKeepsConcurrentReplacement(t *testing.T) {
	e := newEnv(t, newList())
	key := spritestate.TokenKey(defaultAPIURL, "acme")
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{key: "replacement"},
	}))

	require.NoError(t, forgetToken(e.ctx, &session{APIURL: defaultAPIURL, Org: "acme", Token: "rejected"}))
	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, "replacement", st.Tokens[key])
}

func TestFilesystemRequestsHonorCommandCancellation(t *testing.T) {
	for _, op := range []string{"stat", "readdir", "read"} {
		t.Run(op, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(api.Close)
			t.Cleanup(func() { close(release) })
			e := newEnv(t, newFileEdit(), "--sprite", "dev", "/tmp/file")
			t.Setenv("SPRITES_API_URL", api.URL)
			t.Setenv(tokenEnv, "test-token")
			ctx, cancel := context.WithCancel(e.ctx)
			t.Cleanup(cancel)
			s, err := newSession(ctx)
			require.NoError(t, err)
			fsys := s.Client.Sprite("dev").Filesystem()
			done := make(chan error, 1)
			go func() {
				switch op {
				case "stat":
					_, err := fsys.Stat("/tmp/file")
					done <- err
				case "readdir":
					_, err := fsys.ReadDir("/tmp")
					done <- err
				case "read":
					_, err := fsys.ReadFile("/tmp/file")
					done <- err
				}
			}()

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("the filesystem request did not start")
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("the filesystem request did not stop after cancellation")
			}
		})
	}
}

func TestSessionHTTPPreservesExplicitContext(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "signalled")
	}))
	defer api.Close()
	parent, cancel := context.WithCancel(t.Context())
	cancel()

	// Exec uses its own context to send an interrupt after the parent was
	// canceled. The HTTP fallback for that signal must still work.
	ctx, stop := context.WithCancel(context.WithoutCancel(parent))
	defer stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL, nil)
	require.NoError(t, err)
	resp, err := sessionHTTPClient(parent).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
