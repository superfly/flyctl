package server

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/agent/internal/proto"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
)

func setSessionToken(t *testing.T, ctx context.Context, srv *server, kind, value string) *session {
	t.Helper()
	a, b := net.Pipe()
	require.NoError(t, b.SetDeadline(time.Now().Add(5*time.Second)))
	sess := &session{srv: srv, logger: log.New(io.Discard, "", 0), conn: a}
	done := make(chan struct{})
	go func() { defer close(done); sess.setToken(ctx, kind, value) }()
	reply, err := proto.Read(b)
	b.Close()
	<-done
	a.Close()
	require.NoError(t, err)
	require.Equal(t, "ok", string(reply))

	return sess
}

func TestSessionConfigTokens(t *testing.T) {
	t.Chdir(t.TempDir()) // Keep the config lock out of the checkout.
	ctx, cancel := context.WithCancel(logger.NewContext(context.Background(), logger.New(io.Discard, logger.Debug, false)))
	ctx = task.NewWithContext(ctx)
	task.FromContext(ctx).Start(ctx)
	t.Cleanup(func() { cancel(); task.FromContext(ctx).Shutdown() })
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.SetAccessToken(path, "fo1_old"))
	srv := &server{
		Options: Options{Logger: log.New(io.Discard, "", 0)},
		runCtx:  ctx, tokens: tokens.Parse("fo1_env"), slugAliases: map[string]string{},
		cancelTokenMonitoring: func() {},
	}
	// Promote an env-backed agent, then exercise its installed sync below.
	cached, err := srv.tokensFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_old", cached.All())
	require.Equal(t, "fo1_env", srv.GetTokens().All())
	srv.UpdateTokensFromClient(cached)
	require.Same(t, cached, srv.GetTokens())
	require.NoError(t, config.SetAccessToken(path, "fo1_fresh"))

	requests := make(chan string, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"organization":{"id":"test","slug":"test","name":"Test","type":"SHARED","internalNumericId":"1"}}}`)
	}))
	defer api.Close()
	fly.SetBaseURL(api.URL)
	// fly-go's package default is empty; this package has no other override.
	defer fly.SetBaseURL("")

	sess := setSessionToken(t, ctx, srv, "cfg", path)
	_, err = sess.fetchOrg(ctx, "test")
	require.NoError(t, err)
	require.Equal(t, "Bearer fo1_fresh", <-requests)
	require.Same(t, cached, sess.getTokens())

	// A later refresh must be visible to the same session and tunnel selector.
	require.NoError(t, config.SetAccessToken(path, "fo1_later"))
	_, err = srv.tokensFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "Bearer fo1_later", srv.tokensFor(sess.tokens).GraphQLHeader())
	require.Equal(t, "Bearer fo1_later", srv.tokensFor(cached).GraphQLHeader())

	explicit := setSessionToken(t, ctx, srv, "str", "fo1_explicit")
	require.Equal(t, "Bearer fo1_explicit", explicit.getTokens().GraphQLHeader())
	otherPath := filepath.Join(t.TempDir(), "other.yml")
	require.NoError(t, config.SetAccessToken(otherPath, "fo1_other"))
	other := setSessionToken(t, ctx, srv, "cfg", otherPath)
	require.Equal(t, "Bearer fo1_other", other.getTokens().GraphQLHeader())
	require.Equal(t, "Bearer fo1_later", cached.GraphQLHeader())
	require.Same(t, cached, srv.tokensFor(nil))
}

func TestTokenSyncDoesNotHoldServerLock(t *testing.T) {
	cached := tokens.ParseFromFile("fo1_old", "test-config")
	srv := &server{tokens: cached}
	srv.syncTokens = func() error {
		// A waiting writer must not block reconnect readers during file sync.
		require.True(t, srv.tokMu.TryLock(), "file sync must not hold tokMu")
		srv.tokMu.Unlock()

		return nil
	}
	_, err := srv.tokensFromFile("test-config")
	require.NoError(t, err)
}
