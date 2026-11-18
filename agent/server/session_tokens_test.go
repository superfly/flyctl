package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/agent/internal/proto"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/wg"
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
		runCtx:  ctx, tokens: tokens.Parse("fo1_env"),
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

func heldTunnel(name, peerIP string) *wg.Tunnel {
	return &wg.Tunnel{State: &wg.WireGuardState{
		Name:        name,
		LocalPublic: name + "-pub",
		Peer:        fly.CreatedWireGuardPeer{Peerip: peerIP},
	}}
}

// runTunnels runs the tunnels command with args and returns the raw reply.
func runTunnels(t *testing.T, srv *server, args ...string) string {
	t.Helper()

	a, b := net.Pipe()
	require.NoError(t, b.SetDeadline(time.Now().Add(5*time.Second)))

	sess := &session{srv: srv, logger: log.New(io.Discard, "", 0), conn: a}

	done := make(chan struct{})
	go func() { defer close(done); sess.tunnels(context.Background(), args...) }()

	reply, err := proto.Read(b)
	b.Close()
	<-done
	a.Close()
	require.NoError(t, err)

	return string(reply)
}

func TestTunnelsCommand(t *testing.T) {
	srv := &server{
		tunnels: map[tunnelKey]*wg.Tunnel{
			{orgSlug: "jane-doe-123", networkName: "dev"}: heldTunnel("token-bbbbbbbb", "fdaa::b"),
			{orgSlug: "jane-doe-123"}:                     heldTunnel("token-aaaaaaaa", "fdaa::a"),
			{orgSlug: "acme"}:                             heldTunnel("token-cccccccc", "fdaa::c"),
		},
	}

	reply := runTunnels(t, srv, "jane-doe-123")
	require.True(t, strings.HasPrefix(reply, "ok "), reply)

	var got []wg.TunnelStatus
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(reply, "ok ")), &got))

	// the org's tunnels only, default network first
	assert.Equal(t, []wg.TunnelStatus{
		{Org: "jane-doe-123", Name: "token-aaaaaaaa", Pubkey: "token-aaaaaaaa-pub", PeerIP: "fdaa::a"},
		{Org: "jane-doe-123", Network: "dev", Name: "token-bbbbbbbb", Pubkey: "token-bbbbbbbb-pub", PeerIP: "fdaa::b"},
	}, got)

	t.Run("no tunnels", func(t *testing.T) {
		assert.Equal(t, "ok []\n", runTunnels(t, srv, "other-org"))
	})

	t.Run("malformed", func(t *testing.T) {
		assert.Equal(t, "err "+errMalformedTunnels.Error(), runTunnels(t, srv))
	})
}
