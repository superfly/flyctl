package wg

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	t            *testing.T
	srv          *httptest.Server
	pubkeys      []string
	answers      []tokenResult
	silent       bool // accept, then never speak: a legacy gateway waiting for magic
	noAuthHeader bool // hello doesn't advertise reading the Authorization header

	// the n-th in-band refresh is answered with refreshAnswers[n]; later
	// ones go unanswered
	refreshAnswers []tokenResult

	mu      sync.Mutex
	n       int
	auths   []tokenAuthPacket
	headers []string // Authorization header of each accepted connection
	drops   []chan struct{}
	ready   chan struct{} // closed after each accepted connection

	refreshes []tokenRefresh
	refreshed chan struct{} // signaled after each answered refresh
}

func newFakeTokenGateway(t *testing.T, pubkeys []string, answers []tokenResult) *fakeTokenGateway {
	t.Helper()

	g := &fakeTokenGateway{t: t, pubkeys: pubkeys, answers: answers, ready: make(chan struct{}, 16), refreshed: make(chan struct{}, 16)}
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
	g.headers = append(g.headers, r.Header.Get("Authorization"))
	g.mu.Unlock()

	if g.silent {
		<-ctx.Done()

		return
	}

	if n >= len(g.answers) {
		return
	}

	if err := writeJSONFrame(conn, &tokenHello{Version: tokenProtoVersion, Type: "hello", Pubkey: g.pubkeys[n], AuthHeader: !g.noAuthHeader}); err != nil {
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
		defer cancel()

		g.relay(conn)
	}()

	select {
	case <-drop:
	case <-ctx.Done():
	}
}

// relay drains a session's frames, answering refresh packets.
func (g *fakeTokenGateway) relay(conn net.Conn) {
	pbuf := make([]byte, 2000)

	for {
		pkt, err := read(conn, pbuf)
		if err != nil {
			return
		}

		if !isControlFrame(pkt) {
			continue
		}

		var req tokenRefresh
		if err := json.Unmarshal(pkt, &req); err != nil {
			g.t.Errorf("gateway got malformed control packet: %s", err)

			return
		}

		g.mu.Lock()
		n := len(g.refreshes)
		g.refreshes = append(g.refreshes, req)
		g.mu.Unlock()

		if n >= len(g.refreshAnswers) {
			continue // never answered
		}

		if err := writeJSONFrame(conn, &g.refreshAnswers[n]); err != nil {
			return
		}

		g.refreshed <- struct{}{}
	}
}

func (g *fakeTokenGateway) refreshFor(n int) tokenRefresh {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.refreshes[n]
}

func (g *fakeTokenGateway) connections() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.n
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

func (g *fakeTokenGateway) headerFor(n int) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.headers[n]
}

var (
	// directRoutes maps a gateway's direct "ip:443" to the fake gateway
	// serving it; unrouted direct dials fail
	directRoutes sync.Map
	// directDials records the TLS server name each direct "ip:443" was
	// last dialed with
	directDials sync.Map
)

// TestMain points every token-mode test at a plain-text local gateway and
// shortens the proxy's timers. Set once, before any proxy goroutine exists,
// so the race detector doesn't see per-test restores racing with goroutines
// still winding down from a previous test.
func TestMain(m *testing.M) {
	dialWebsocket = func(dialCtx, lifetimeCtx context.Context, endpoint, serverName string, verifyTLS bool, authorization string) (net.Conn, error) {
		// direct dials arrive as ip:443; fake gateways listen on random
		// ports, so tests route them here
		if _, port, _ := net.SplitHostPort(endpoint); port == "443" {
			directDials.Store(endpoint, serverName)

			to, ok := directRoutes.Load(endpoint)
			if !ok {
				return nil, errors.New("connection refused")
			}
			endpoint = to.(string)
		}

		header := http.Header{}
		if authorization != "" {
			header.Set("Authorization", authorization)
		}

		ws, _, err := websocket.Dial(dialCtx, "ws://"+endpoint+"/", &websocket.DialOptions{HTTPHeader: header}) // nolint: bodyclose
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
	// the macaroon rides in the upgrade request, for proxies to act on; the
	// gateway advertised reading it there, so the packet carries no copy
	assert.Equal(t, "FlyV1 fm2_test", gateway.headerFor(0))
	assert.Equal(t, "", auth.Token)

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
	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: expiry.Unix()},
	})
	gateway.refreshAnswers = []tokenResult{
		{Type: "refreshed", ExpiresAt: expiry.Unix()},
		{Type: "refreshed", ExpiresAt: expiry.Add(time.Hour).Unix()},
	}

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
	case <-gateway.refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy never re-presented the unchanged token")
	}

	select {
	case <-gateway.refreshed:
		t.Fatal("proxy re-presented the unchanged token again for the same expiry")
	case <-time.After(2 * time.Second):
	}
}

func TestConnectTokenSameTokenExtendsSessionCap(t *testing.T) {
	gw, _ := testKeypair(t)
	const peer = "fdaa:0:18:a7b:1:1111:2222:3302"

	// a token with no validity window: every expiry is the gateway's
	// session cap, and re-presenting the same token keeps extending it
	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(2 * time.Second).Unix()},
	})
	gateway.refreshAnswers = []tokenResult{
		{Type: "refreshed", ExpiresAt: time.Now().Add(4 * time.Second).Unix()},
		{Type: "refreshed", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_same" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	for i := 0; i < 2; i++ {
		select {
		case <-gateway.refreshed:
		case <-time.After(6 * time.Second):
			t.Fatalf("only %d of 2 refreshes answered", i)
		}
	}

	waitFor(t, "extended expiry", func() bool {
		return tunnel.Provision().ExpiresAt.After(time.Now().Add(30 * time.Minute))
	})

	assert.Equal(t, 1, gateway.connections())
	assert.Equal(t, peer, tunnel.Provision().PeerIP)

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

func TestConnectTokenPacketTokenForOlderGateway(t *testing.T) {
	gw, _ := testKeypair(t)
	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})
	gateway.noAuthHeader = true

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-gateway.ready

	// a gateway that doesn't read the header still gets the token, in the
	// packet, and the header does no harm
	assert.Equal(t, "FlyV1 fm2_test", gateway.authFor(0).Token)
	assert.Equal(t, "FlyV1 fm2_test", gateway.headerFor(0))
}

func TestConnectTokenReprovisionClosesOpenConns(t *testing.T) {
	gw1, _ := testKeypair(t)
	gw2, _ := testKeypair(t)

	gateway := newFakeTokenGateway(t, []string{gw1, gw2}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		{Type: "ok", PeerIP: "fdaa:0:18:c9d:2:3333:4444:5502", DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix()},
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

	// a connection to our own address stays inside the netstack, standing
	// in for an ssh session to a machine behind the gateway
	gNet, _, err := tunnel.netstack()
	require.NoError(t, err)

	_, cfg := tunnel.StateAndConfig()
	ln, err := gNet.ListenTCP(&net.TCPAddr{IP: cfg.LocalNetwork.IP, Port: 2222})
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			_, _ = io.Copy(io.Discard, c)
		}
	}()

	conn, err := tunnel.DialContext(context.Background(), "tcp", net.JoinHostPort(cfg.LocalNetwork.IP.String(), "2222"))
	require.NoError(t, err)
	defer conn.Close()

	readErr := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		readErr <- err
	}()

	// anycast moved us: the connection's source address is gone
	gateway.drop(0)

	select {
	case err := <-readErr:
		assert.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("read through the old netstack still blocked after the tunnel was rebuilt")
	}

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}

// connectRefreshing connects to gateway with a token that the test can
// replace, as the agent's token monitor does.
func connectRefreshing(t *testing.T, gateway *fakeTokenGateway) (*Tunnel, func(string)) {
	t.Helper()

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
	t.Cleanup(func() { tunnel.Close() })

	<-gateway.ready

	return tunnel, func(tok string) {
		mu.Lock()
		token = tok
		mu.Unlock()
	}
}

func TestConnectTokenRefreshesInBand(t *testing.T) {
	gw, _ := testKeypair(t)
	const peer = "fdaa:0:18:a7b:1:1111:2222:3302"

	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "ok", PeerIP: peer, ExpiresAt: time.Now().Add(2 * time.Second).Unix()},
	})
	gateway.refreshAnswers = []tokenResult{
		{Type: "refreshed", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}

	tunnel, setToken := connectRefreshing(t, gateway)
	setToken("FlyV1 fm2_v2")

	select {
	case <-gateway.refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not refresh before expiry")
	}

	assert.Equal(t, tokenRefresh{Version: tokenProtoVersion, Type: "refresh", Token: "FlyV1 fm2_v2"}, gateway.refreshFor(0))

	waitFor(t, "provision update", func() bool {
		return tunnel.Provision().ExpiresAt.After(time.Now().Add(30 * time.Minute))
	})

	// extended on the same connection: no reconnect, so no chance of
	// landing on another gateway
	assert.Equal(t, 1, gateway.connections())
	assert.Equal(t, peer, tunnel.Provision().PeerIP)

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}

func TestConnectTokenInBandRefreshRetriesTransientFailure(t *testing.T) {
	gw, _ := testKeypair(t)

	gateway := newFakeTokenGateway(t, []string{gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(3 * time.Second).Unix()},
	})
	gateway.refreshAnswers = []tokenResult{
		{Type: "error", Code: GatewayErrTransient, Error: "tkdb unreachable"},
		{Type: "refreshed", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}

	tunnel, setToken := connectRefreshing(t, gateway)
	setToken("FlyV1 fm2_v2")

	for i := 0; i < 2; i++ {
		select {
		case <-gateway.refreshed:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of 2 refreshes answered", i)
		}
	}

	waitFor(t, "provision update", func() bool {
		return tunnel.Provision().ExpiresAt.After(time.Now().Add(30 * time.Minute))
	})

	assert.Equal(t, 1, gateway.connections())
}

func TestConnectTokenInBandRefreshRejectedReconnects(t *testing.T) {
	gw, _ := testKeypair(t)

	// the refresh is rejected for good; the reconnect's full exchange gives
	// the verdict, and here it stands
	gateway := newFakeTokenGateway(t, []string{gw, gw}, []tokenResult{
		{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", ExpiresAt: time.Now().Add(2 * time.Second).Unix()},
		{Type: "error", Code: GatewayErrUnauthorized, Error: "token revoked"},
	})
	gateway.refreshAnswers = []tokenResult{
		{Type: "error", Code: GatewayErrUnauthorized, Error: "token revoked"},
	}

	tunnel, setToken := connectRefreshing(t, gateway)
	setToken("FlyV1 fm2_v2")

	select {
	case <-tunnel.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("tunnel survived a rejected token")
	}

	var gwErr *GatewayError
	require.ErrorAs(t, tunnel.Err(), &gwErr)
	assert.Equal(t, GatewayErrUnauthorized, gwErr.Code)
	assert.Equal(t, 2, gateway.connections())
}

// routeDirect sends dials to ip:443 to gw for the rest of the test.
func routeDirect(t *testing.T, ip string, gw *fakeTokenGateway) {
	t.Helper()

	addr := net.JoinHostPort(ip, "443")
	directRoutes.Store(addr, gw.endpoint())
	t.Cleanup(func() {
		directRoutes.Delete(addr)
		directDials.Delete(addr)
	})
}

func TestConnectTokenReconnectsToDirectIP(t *testing.T) {
	gw, _ := testKeypair(t)

	const (
		peerA  = "fdaa:0:18:a7b:1:1111:2222:3302"
		direct = "192.0.2.10"
	)

	ok := tokenResult{Type: "ok", PeerIP: peerA, DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix(), Direct: direct}

	// the anycast endpoint would pick some gateway; the direct IP reaches
	// the one we're on. Anycast answers twice: the first connect, and the
	// fallback once the direct IP stops answering.
	anycast := newFakeTokenGateway(t, []string{gw, gw}, []tokenResult{ok, ok})
	edge := newFakeTokenGateway(t, []string{gw}, []tokenResult{ok})
	routeDirect(t, direct, edge)

	pub, priv := testKeypair(t)
	state := &WireGuardState{Org: "test-org", LocalPublic: pub, LocalPrivate: priv}

	tunnel, err := ConnectToken(context.Background(), state, anycast.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	defer tunnel.Close()

	<-anycast.ready
	assert.Equal(t, direct, tunnel.Provision().Direct)

	anycast.drop(0)
	<-edge.ready

	// TLS verifies the gateway's name, not the IP we dialed
	serverName, dialed := directDials.Load(net.JoinHostPort(direct, "443"))
	require.True(t, dialed)
	anycastHost, _, err := net.SplitHostPort(anycast.endpoint())
	require.NoError(t, err)
	assert.Equal(t, anycastHost, serverName)

	assert.Equal(t, 1, anycast.connections(), "reconnect went to the anycast endpoint")
	assert.Equal(t, "FlyV1 fm2_test", edge.headerFor(0))

	st, _ := tunnel.StateAndConfig()
	assert.Equal(t, peerA, st.Peer.Peerip)

	// the direct IP stops answering: the next reconnect tries it, then
	// falls back to the anycast endpoint
	directAddr := net.JoinHostPort(direct, "443")
	directDials.Delete(directAddr)
	directRoutes.Delete(directAddr)

	edge.drop(0)
	<-anycast.ready

	_, dialed = directDials.Load(directAddr)
	assert.True(t, dialed, "reconnect didn't try the direct IP first")
	assert.Equal(t, 1, edge.connections())
	assert.Equal(t, 2, anycast.connections(), "reconnect didn't fall back to the anycast endpoint")

	st, _ = tunnel.StateAndConfig()
	assert.Equal(t, peerA, st.Peer.Peerip)

	select {
	case <-tunnel.Done():
		t.Fatalf("tunnel died: %v", tunnel.Err())
	default:
	}
}
