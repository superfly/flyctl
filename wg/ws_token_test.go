package wg

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/curve25519"
)

func testKeypair(t *testing.T) (pub, priv string) {
	t.Helper()

	var private [32]byte
	_, err := rand.Read(private[:])
	require.NoError(t, err)

	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(public), base64.StdEncoding.EncodeToString(private[:])
}

// fakeTokenGateway serves the token wswg protocol over plain websockets.
// Each accepted connection is answered by answers[n] (n counting from 0);
// an "ok" answer is followed by draining the relay until the gateway is
// told to drop the connection or the test ends.
type fakeTokenGateway struct {
	t       *testing.T
	srv     *httptest.Server
	pubkeys []string
	answers []tokenResult
	silent  bool // accept, then never speak: a legacy gateway waiting for magic

	mu    sync.Mutex
	n     int
	auths []tokenAuthPacket
	drops []chan struct{}
	ready chan struct{} // closed after each accepted connection
}

func newFakeTokenGateway(t *testing.T, pubkeys []string, answers []tokenResult) *fakeTokenGateway {
	t.Helper()

	g := &fakeTokenGateway{t: t, pubkeys: pubkeys, answers: answers, ready: make(chan struct{}, 16)}
	g.srv = httptest.NewServer(http.HandlerFunc(g.handle))
	t.Cleanup(g.srv.Close)

	return g
}

func (g *fakeTokenGateway) endpoint() string {
	return g.srv.Listener.Addr().String()
}

func (g *fakeTokenGateway) handle(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn := websocket.NetConn(ctx, ws, websocket.MessageText)
	defer conn.Close()

	g.mu.Lock()
	n := g.n
	g.n++
	drop := make(chan struct{})
	g.drops = append(g.drops, drop)
	g.mu.Unlock()

	if g.silent {
		<-ctx.Done()

		return
	}

	if n >= len(g.answers) {
		return
	}

	if err := writeJSONFrame(conn, &tokenHello{Version: tokenProtoVersion, Type: "hello", Pubkey: g.pubkeys[n]}); err != nil {
		return
	}

	var auth tokenAuthPacket
	if err := readJSONFrame(conn, &auth); err != nil {
		return
	}

	g.mu.Lock()
	g.auths = append(g.auths, auth)
	g.mu.Unlock()

	if err := writeJSONFrame(conn, &g.answers[n]); err != nil {
		return
	}

	g.ready <- struct{}{}

	if g.answers[n].Type != "ok" {
		return
	}

	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
	}()

	select {
	case <-drop:
	case <-ctx.Done():
	}
}

// drop closes the n-th accepted connection from the gateway side.
func (g *fakeTokenGateway) drop(n int) {
	g.mu.Lock()
	ch := g.drops[n]
	g.mu.Unlock()

	close(ch)
}

func (g *fakeTokenGateway) authFor(n int) tokenAuthPacket {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.auths[n]
}

// TestMain points every token-mode test at a plain-text local gateway and
// shortens the proxy's timers. Set once, before any proxy goroutine exists,
// so the race detector doesn't see per-test restores racing with goroutines
// still winding down from a previous test.
func TestMain(m *testing.M) {
	dialWebsocket = func(dialCtx, lifetimeCtx context.Context, endpoint string, verifyTLS bool) (net.Conn, error) {
		ws, _, err := websocket.Dial(dialCtx, "ws://"+endpoint+"/", nil) // nolint: bodyclose
		if err != nil {
			return nil, err
		}

		return websocket.NetConn(lifetimeCtx, ws, websocket.MessageText), nil
	}
	reconnectInterval = 100 * time.Millisecond
	tokenRefreshMargin = 500 * time.Millisecond
	tokenRefreshRetry = 200 * time.Millisecond
	tokenHelloTimeout = 2 * time.Second

	os.Exit(m.Run())
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestConnectTokenReprovisionRebuildsTunnel(t *testing.T) {
	gw1, _ := testKeypair(t)
	gw2, _ := testKeypair(t)

	const (
		peerA = "fdaa:0:18:a7b:1:1111:2222:3302"
		peerB = "fdaa:0:18:c9d:2:3333:4444:5502"
	)

	gateway := newFakeTokenGateway(t, []string{gw1, gw2}, []tokenResult{
		{Type: "ok", PeerIP: peerA, DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		{Type: "ok", PeerIP: peerB, DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:       func() string { return "FlyV1 fm2_test" },
		OrgSlug:     "test-org",
		NetworkName: "custom",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	assert.True(t, tunnel.Ephemeral())
	assert.Equal(t, TokenPeerName(pub), state.Name)

	auth := gateway.authFor(0)
	assert.Equal(t, pub, auth.Pubkey)
	assert.Equal(t, "test-org", auth.OrgSlug)
	assert.Equal(t, "custom", auth.NetworkName)
	assert.Equal(t, "FlyV1 fm2_test", auth.Token)

	st, cfg := tunnel.StateAndConfig()
	assert.Equal(t, peerA, st.Peer.Peerip)
	assert.Equal(t, gw1, st.Peer.Pubkey)
	// the device binds the peer's /120 network address, as legacy tunnels do
	assert.Equal(t, "fdaa:0:18:a7b:1:1111:2222:3300/120", cfg.LocalNetwork.String())
	assert.Equal(t, "fdaa:0:18::3", cfg.DNS.String())

	// anycast moved us: the reconnect lands on another gateway
	gateway.drop(0)
	<-gateway.ready

	waitFor(t, "tunnel rebuild", func() bool {
		st, _ := tunnel.StateAndConfig()

		return st.Peer.Peerip == peerB
	})

	st, cfg = tunnel.StateAndConfig()
	assert.Equal(t, gw2, st.Peer.Pubkey)
	assert.Equal(t, "fdaa:0:18:c9d:2:3333:4444:5500/120", cfg.LocalNetwork.String())
	assert.Equal(t, peerB, tunnel.Provision().PeerIP)
	assert.Equal(t, gw2, tunnel.Provision().GatewayPubkey)

	// the rebuilt netstack is usable
	_, err = tunnel.ListenPing()
	assert.NoError(t, err)

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}

func TestConnectTokenRejectedOnReconnectKillsTunnel(t *testing.T) {
	gw, _ := testKeypair(t)

	gateway := newFakeTokenGateway(t, []string{gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		{Type: "error", Code: GatewayErrUnauthorized, Error: "token not authorized for wireguard access"},
	})

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	// token revoked: the gateway tears the session down
	gateway.drop(0)
	<-gateway.ready

	select {
	case <-tunnel.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("tunnel did not die after being rejected")
	}

	var gwErr *GatewayError
	require.True(t, errors.As(tunnel.Err(), &gwErr), "err: %v", tunnel.Err())
	assert.Equal(t, GatewayErrUnauthorized, gwErr.Code)

	_, err = tunnel.DialContext(context.Background(), "tcp", "[fdaa:0:18::3]:53")
	assert.ErrorIs(t, err, errTunnelClosed)
}

func TestConnectTokenRejectedInitially(t *testing.T) {
	gw, _ := testKeypair(t)
	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "error", Code: GatewayErrInvalidRequest, Error: "org \"nope\": not found"},
	})

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "nope", LocalPublic: pub, LocalPrivate: priv}

	_, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		OrgSlug: "nope",
	})
	require.Error(t, err)

	var gwErr *GatewayError
	require.True(t, errors.As(err, &gwErr), "err: %v", err)
	assert.Equal(t, GatewayErrInvalidRequest, gwErr.Code)
}

func TestConnectTokenRefreshesBeforeExpiry(t *testing.T) {
	gw, _ := testKeypair(t)
	const peer = "fdaa:0:18:a7b:1:1111:2222:3302"

	// the first session is short-lived; re-presenting the (refreshed) token
	// extends it without the peer changing
	gateway := newFakeTokenGateway(t, []string{gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(2 * time.Second).Unix()},
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})

	var mu sync.Mutex
	token := "FlyV1 fm2_v1"

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token: func() string {
			mu.Lock()
			defer mu.Unlock()

			return token
		},
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	// the agent's token monitor refreshed the discharge in the meantime
	mu.Lock()
	token = "FlyV1 fm2_v2"
	mu.Unlock()

	select {
	case <-gateway.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not re-present the token before expiry")
	}

	assert.Equal(t, "FlyV1 fm2_v2", gateway.authFor(1).Token)

	waitFor(t, "provision update", func() bool {
		return tunnel.Provision().ExpiresAt.After(time.Now().Add(30 * time.Minute))
	})

	// same peer: the device was kept, not rebuilt
	st, _ := tunnel.StateAndConfig()
	assert.Same(t, state, st)
	assert.Equal(t, peer, st.Peer.Peerip)

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}

func TestConnectTokenSilentGateway(t *testing.T) {
	gateway := newFakeTokenGateway(t, nil, nil)
	gateway.silent = true

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	start := time.Now()
	_, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		OrgSlug: "test-org",
	})
	require.Error(t, err)
	// through a real websocket conn the deadline surfaces as a context
	// cancellation; it must still be recognized as "no hello"
	assert.ErrorIs(t, err, ErrNoHello, "got: %v", err)
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestConnectTokenSameTokenRefreshedOncePerWindow(t *testing.T) {
	gw, _ := testKeypair(t)
	expiry := time.Now().Add(3 * time.Second).Truncate(time.Second)

	// the expiry might be the gateway's session cap, so the unchanged token
	// is re-presented once; when that yields the same expiry it's the
	// token's own limit and there's nothing more to gain
	gateway := newFakeTokenGateway(t, []string{gw, gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: expiry.Unix()},
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: expiry.Unix()},
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: expiry.Add(time.Hour).Unix()},
	})

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_same" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	select {
	case <-gateway.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy never re-presented the unchanged token")
	}

	select {
	case <-gateway.ready:
		t.Fatal("proxy re-presented the unchanged token again for the same expiry")
	case <-time.After(2 * time.Second):
	}
}

func TestConnectTokenSameTokenExtendsSessionCap(t *testing.T) {
	gw, _ := testKeypair(t)
	const peer = "fdaa:0:18:a7b:1:1111:2222:3302"

	// a token with no validity window: every expiry is the gateway's
	// session cap, and re-presenting the same token keeps extending it
	gateway := newFakeTokenGateway(t, []string{gw, gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(2 * time.Second).Unix()},
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(4 * time.Second).Unix()},
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_same" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	for i := 0; i < 3; i++ {
		select {
		case <-gateway.ready:
		case <-time.After(6 * time.Second):
			t.Fatalf("only %d of 3 sessions established", i)
		}
	}

	waitFor(t, "extended expiry", func() bool {
		return tunnel.Provision().ExpiresAt.After(time.Now().Add(30 * time.Minute))
	})

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}

func TestWebsocketURLKeepsExplicitPort(t *testing.T) {
	assert.Equal(t, "wss://gateway.machines.dev:443/", websocketURL("gateway.machines.dev"))
	assert.Equal(t, "wss://localhost:8443/", websocketURL("localhost:8443"))
	assert.Equal(t, "wss://[::1]:8443/", websocketURL("[::1]:8443"))
}

func TestConnectTokenNoTokenOnReconnectKillsTunnel(t *testing.T) {
	gw, _ := testKeypair(t)
	gateway := newFakeTokenGateway(t, []string{gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})

	var mu sync.Mutex
	token := "FlyV1 fm2_test"

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token: func() string {
			mu.Lock()
			defer mu.Unlock()

			return token
		},
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	// the tokens were removed from the config file while the tunnel was up
	mu.Lock()
	token = ""
	mu.Unlock()

	gateway.drop(0)

	select {
	case <-tunnel.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("tunnel with no token to present kept retrying instead of dying")
	}

	assert.ErrorIs(t, tunnel.Err(), ErrNoToken)
}
