package wg

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
)

func ConnectWS(ctx context.Context, state *WireGuardState) (*Tunnel, error) {
	return doConnect(ctx, state, true)
}

func read(r io.Reader, rbuf []byte) ([]byte, error) {
	var lbuf [4]byte
	if _, err := io.ReadFull(r, lbuf[:]); err != nil {
		return nil, err
	}

	plen := binary.BigEndian.Uint32(lbuf[:])
	if plen >= uint32(len(rbuf)) {
		rbuf = make([]byte, plen)
	}

	if _, err := io.ReadFull(r, rbuf[:plen]); err != nil {
		return nil, err
	}

	return rbuf[:plen], nil
}

type WsWgProxy struct {
	wsConn       net.Conn
	plugConn     *net.UDPConn
	lastPlugAddr net.Addr
	lock         sync.RWMutex
	wrlock       sync.Mutex
	atime        time.Time
	reset        chan bool
	limit        *rate.Limiter

	// token-authenticated mode: instead of the legacy magic, each
	// (re-)connection authenticates with a macaroon and the gateway
	// provisions our peer inline
	auth *TokenAuth
	prov *TokenProvision

	// refreshAt is when to reconnect next to extend the gateway session
	// with a refreshed token; zero when there's nothing to extend.
	refreshAt time.Time

	// sameTokenTriedFor is the session expiry for which the unchanged
	// token has already been re-presented, so it's tried once per window.
	sameTokenTriedFor time.Time

	// onReprovision fires (from Connect, with lock held) when a reconnect
	// landed us on a different peer address or gateway key, so the owner
	// can rebuild the WireGuard device around the new addresses. onFatal
	// fires when the gateway rejects us for good. Neither may call back
	// into the proxy.
	onReprovision func(*TokenProvision)
	onFatal       func(error)
}

var (
	// reconnectInterval paces reconnection attempts after a websocket
	// drops. A variable so tests don't have to wait.
	reconnectInterval = 5 * time.Second

	// tokenRefreshMargin is how long before the gateway's expiry a
	// token-mode proxy reconnects to present a refreshed token. The agent
	// refreshes discharges between two and one minutes ahead of their
	// expiry, so by 45s out a fresh token is normally in hand. Re-presenting
	// the same pubkey keeps the peer address; only the session's expiry
	// moves, so nothing else changes.
	tokenRefreshMargin = 45 * time.Second

	// tokenRefreshRetry floors the time between refresh attempts when the
	// token hasn't actually been refreshed yet.
	tokenRefreshRetry = 15 * time.Second
)

// dialWebsocket dials the gateway's wswg endpoint and wraps it as a
// net.Conn bound to lifetimeCtx. A non-empty authorization goes out as the
// upgrade request's Authorization header. A variable so tests can point it
// at a plain-text local server.
var dialWebsocket = func(dialCtx, lifetimeCtx context.Context, endpoint string, verifyTLS bool, authorization string) (net.Conn, error) {
	rurl := websocketURL(endpoint)

	log.Printf("(re-)connecting to %s", rurl)

	header := http.Header{
		"Origin": []string{rurl},
	}
	if authorization != "" {
		header.Set("Authorization", authorization)
	}

	ws, _, err := websocket.Dial(dialCtx, rurl, &websocket.DialOptions{ // nolint: bodyclose
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				// Legacy gateways serve a self-signed cert; the tunnel traffic is
				// WG-encrypted anyway. Token gateways have a real cert for their
				// hostname and we hand them a macaroon, so verify those.
				TLSClientConfig: &tls.Config{ // skipcq: GO-S1020
					InsecureSkipVerify: !verifyTLS, // skipcq: GSC-G402
				},
			},
		},
		HTTPHeader: header,
	})
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}

	return websocket.NetConn(lifetimeCtx, ws, websocket.MessageText), nil
}

// websocketURL builds the gateway's wswg URL; endpoint is a hostname, or
// host:port for gateways not on 443.
func websocketURL(endpoint string) string {
	host := endpoint
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		host = net.JoinHostPort(endpoint, "443")
	}

	return (&url.URL{
		Scheme: "wss",
		Host:   host,
		Path:   "/",
	}).String()
}

// this is gross, but, keep the rest of the WireGuard code in
// flyctl oblivious to the fact that we're potentially proxying
// it over tcp.

func NewWsWgProxy() (*WsWgProxy, error) {
	laddr := net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	}

	l, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, fmt.Errorf("start wswg: %w", err)
	}

	return &WsWgProxy{
		atime:    time.Now(),
		plugConn: l,
		reset:    make(chan bool),
		limit:    rate.NewLimiter(rate.Every(5*time.Second), 2),
	}, nil
}

func (wswg *WsWgProxy) touch() {
	wswg.lock.Lock()
	wswg.atime = time.Now()
	wswg.lock.Unlock()
}

func (wswg *WsWgProxy) lastIo() time.Duration {
	wswg.lock.RLock()
	s := time.Since(wswg.atime)
	wswg.lock.RUnlock()

	return s
}

func (wswg *WsWgProxy) resetConn(ctx context.Context, c net.Conn, err error) {
	wswg.lock.RLock()
	cur := wswg.wsConn
	wswg.lock.RUnlock()

	if cur != c || ctx.Err() != nil {
		return
	}

	if err := wswg.limit.Wait(ctx); err != nil {
		return
	}

	log.Printf("resetting connection due to error: %s", err)

	select {
	case wswg.reset <- true:
	case <-ctx.Done():
	}
}

func (wswg *WsWgProxy) Port() (int, error) {
	bindAddr := wswg.plugConn.LocalAddr()
	udpBindAddr, ok := bindAddr.(*net.UDPAddr)
	if !ok {
		return 0, fmt.Errorf("plugboard: can't recover UDP port")
	}

	log.Printf("returning port: %d", udpBindAddr.Port)

	return udpBindAddr.Port, nil
}

func (wswg *WsWgProxy) Connect(dialCtx, lifetimeCtx context.Context, endpoint string) error {
	// one token per connection attempt: the same one goes in the upgrade
	// request's Authorization header and the auth packet
	token := wswg.auth.currentToken()
	if wswg.auth != nil && token == "" {
		if wswg.onFatal != nil {
			wswg.onFatal(ErrNoToken)
		}

		return fmt.Errorf("token exchange: %w", ErrNoToken)
	}

	wsConn, err := dialWebsocket(dialCtx, lifetimeCtx, endpoint, wswg.auth != nil, token)
	if err != nil {
		return err
	}

	var reprovisioned *TokenProvision

	if wswg.auth != nil {
		prov, err := tokenExchange(wsConn, wswg.auth, token)
		if err != nil {
			_ = wsConn.Close()

			if permanentTokenFailure(err) && wswg.onFatal != nil {
				wswg.onFatal(err)
			}

			return fmt.Errorf("token exchange: %w", err)
		}

		if wswg.prov != nil && !wswg.prov.same(prov) {
			// Anycast: the reconnect landed on another edge, which has its
			// own key and allocated us a fresh address. Frames from the new
			// peer must not be relayed to the old device, which the owner is
			// about to replace.
			log.Printf("gateway re-provisioned our peer (%s -> %s)", wswg.prov.PeerIP, prov.PeerIP)

			reprovisioned = prov
			wswg.lastPlugAddr = nil
		} else if wswg.prov != nil {
			log.Printf("gateway session re-authenticated: peer %s, expires %s", prov.PeerIP, prov.ExpiresAt.Format(time.RFC3339))
		}
		wswg.prov = prov
		wswg.scheduleRefresh(prov)
	} else {
		var magic [4]byte
		binary.BigEndian.PutUint32(magic[:], 0x2FACED77)

		if _, err = wsConn.Write(magic[:]); err != nil {
			_ = wsConn.Close()

			return fmt.Errorf("write websocket magic: %w", err)
		}
	}

	if wswg.wsConn != nil {
		_ = wswg.wsConn.Close()
	}
	wswg.wsConn = wsConn

	if reprovisioned != nil && wswg.onReprovision != nil {
		wswg.onReprovision(reprovisioned)
	}

	return nil
}

// permanentTokenFailure reports whether reconnecting with the same
// credentials can't help: the gateway rejected the token or request for
// good, there's no token to present at all, or the gateway speaks
// something we can't use.
func permanentTokenFailure(err error) bool {
	var gwErr *GatewayError

	return (errors.As(err, &gwErr) && gwErr.Permanent()) ||
		errors.Is(err, ErrNoToken) ||
		errors.Is(err, ErrMalformedReply)
}

// close releases the proxy's sockets; for a proxy that was never started.
func (wswg *WsWgProxy) close() {
	if wswg.wsConn != nil {
		_ = wswg.wsConn.Close()
	}
	_ = wswg.plugConn.Close()
}

// scheduleRefresh arranges to re-present our token before the gateway's
// session expiry, so a refreshed token extends the session instead of the
// gateway tearing the peer down.
func (wswg *WsWgProxy) scheduleRefresh(prov *TokenProvision) {
	remaining := time.Until(prov.ExpiresAt)
	if remaining <= 0 {
		wswg.refreshAt = time.Time{}

		return
	}

	margin := tokenRefreshMargin
	if remaining < 2*margin {
		margin = remaining / 2
	}

	wswg.refreshAt = prov.ExpiresAt.Add(-margin)
	if floor := time.Now().Add(tokenRefreshRetry); wswg.refreshAt.Before(floor) {
		wswg.refreshAt = floor
	}
}

// provision returns the latest gateway answer.
func (wswg *WsWgProxy) provision() *TokenProvision {
	wswg.lock.RLock()
	defer wswg.lock.RUnlock()

	return wswg.prov
}

// setCallbacks installs the token-mode lifecycle hooks; see the field docs.
func (wswg *WsWgProxy) setCallbacks(onReprovision func(*TokenProvision), onFatal func(error)) {
	wswg.lock.Lock()
	defer wswg.lock.Unlock()

	wswg.onReprovision = onReprovision
	wswg.onFatal = onFatal
}

func isTimeout(e error) bool {
	var err net.Error

	return errors.As(e, &err) && err.Timeout()
}

func (wswg *WsWgProxy) wsWrite(c net.Conn, b []byte) error {
	wswg.wrlock.Lock()
	defer wswg.wrlock.Unlock()

	_, err := c.Write(b)

	return err
}

func (wswg *WsWgProxy) ws2wg(ctx context.Context) {
	pbuf := make([]byte, 2000)

	for ctx.Err() == nil {
		wswg.lock.RLock()
		c := wswg.wsConn
		wswg.lock.RUnlock()

		pkt, err := read(c, pbuf)
		if err != nil {
			wswg.resetConn(ctx, c, err)

			continue
		}

		wswg.touch()

		wswg.lock.RLock()
		addr := wswg.lastPlugAddr
		wswg.lock.RUnlock()

		// On token gateways the kernel peer exists (and sends keepalives)
		// before our wg device has spoken, so frames can arrive before we
		// know where to deliver them. Drop them; wg retransmits handshakes.
		if addr == nil {
			continue
		}

		if _, err = wswg.plugConn.WriteTo(pkt, addr); err != nil {
			wswg.resetConn(ctx, c, err)
		}
	}
}

func (wswg *WsWgProxy) wg2ws(ctx context.Context) {
	var buf [2000]byte

	for ctx.Err() == nil {
		wswg.plugConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, a, err := wswg.plugConn.ReadFrom(buf[4:])
		if err != nil {
			if isTimeout(err) {
				continue
			}

			// resetting won't do anything here
			log.Printf("error reading from udp plugboard: %s", err)
		}
		binary.BigEndian.PutUint32(buf[:], uint32(n))

		wswg.lock.Lock()
		wswg.lastPlugAddr = a
		c := wswg.wsConn
		wswg.lock.Unlock()

		if err = wswg.wsWrite(c, buf[:n+4]); err != nil {
			wswg.resetConn(ctx, c, err)
		}

		wswg.touch()
	}
}

func websocketConnect(dialCtx, lifetimeCtx context.Context, endpoint string) (int, error) {
	wswg, err := websocketConnectAuth(dialCtx, lifetimeCtx, endpoint, nil)
	if err != nil {
		return 0, err
	}

	wswg.start(lifetimeCtx, endpoint)

	return wswg.Port()
}

// websocketConnectAuth dials the gateway once, with optional token
// authentication (a non-nil auth makes every (re-)connection perform the
// token exchange; wswg.prov holds the latest provision). Nothing runs yet:
// the caller inspects the proxy, installs callbacks, and then calls start.
func websocketConnectAuth(dialCtx, lifetimeCtx context.Context, endpoint string, auth *TokenAuth) (*WsWgProxy, error) {
	wswg, err := NewWsWgProxy()
	if err != nil {
		return nil, err
	}
	wswg.auth = auth

	if err = wswg.Connect(dialCtx, lifetimeCtx, endpoint); err != nil {
		wswg.plugConn.Close()

		return nil, err
	}

	return wswg, nil
}

// start runs the relay, reconnect and keepalive loops until lifetimeCtx is
// canceled.
func (wswg *WsWgProxy) start(lifetimeCtx context.Context, endpoint string) {
	go func() {
		defer wswg.wsConn.Close()   // skipcq: GO-S2307
		defer wswg.plugConn.Close() // skipcq: GO-S2307

		c := make(chan os.Signal, 1)
		signalChannel(c)

		tick := time.NewTicker(reconnectInterval)
		defer tick.Stop()

		reconnectAt := time.Time{}

		for {
			select {
			case <-tick.C:
				now := time.Now()

				wswg.lock.RLock()
				refreshAt, prov := wswg.refreshAt, wswg.prov
				wswg.lock.RUnlock()

				if !refreshAt.IsZero() && refreshAt.Before(now) && reconnectAt.IsZero() {
					wswg.lock.Lock()
					sameToken := wswg.auth.Token() == prov.token
					triedAlready := sameToken && wswg.sameTokenTriedFor.Equal(prov.ExpiresAt)
					if triedAlready {
						// The unchanged token already got us this expiry:
						// it's the token's own limit. Wait for a new one.
						wswg.refreshAt = now.Add(tokenRefreshRetry)
					} else if sameToken {
						// The expiry may be the gateway's session cap rather
						// than the token's: re-presenting the same token
						// then extends it. Try once per window.
						wswg.sameTokenTriedFor = prov.ExpiresAt
					}
					wswg.lock.Unlock()

					if !triedAlready {
						log.Printf("re-presenting token to extend gateway session (expires %s)", prov.ExpiresAt.Format(time.RFC3339))

						reconnectAt = now
					}
				}

				if !reconnectAt.IsZero() && !reconnectAt.After(now) {
					wswg.lock.Lock()
					wswg.refreshAt = time.Time{}
					if err := wswg.Connect(lifetimeCtx, lifetimeCtx, endpoint); err != nil {
						// After a dropped connection the relay loops keep
						// failing on the dead conn and ask for another reset.
						// After a failed refresh the old conn is still fine;
						// try again before the session expires.
						log.Printf("reconnect failed: %s", err)

						if wswg.prov != nil {
							wswg.scheduleRefresh(wswg.prov)
						}
					}
					wswg.lock.Unlock()

					reconnectAt = time.Time{}
				}

			case <-lifetimeCtx.Done():
				return

			case <-c:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(reconnectInterval)
				}

			case <-wswg.reset:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(reconnectInterval)
				}
			}
		}
	}()

	go func() {
		go wswg.ws2wg(lifetimeCtx)
		go wswg.wg2ws(lifetimeCtx)

		zeroLenMsg := make([]byte, 4)

		for lifetimeCtx.Err() == nil {
			time.Sleep(1 * time.Second)

			if wswg.lastIo() > (1 * time.Second) {
				wswg.lock.RLock()
				c := wswg.wsConn
				wswg.lock.RUnlock()

				if err := wswg.wsWrite(c, zeroLenMsg); err != nil {
					wswg.resetConn(lifetimeCtx, c, err)
				}
			}
		}
	}()
}
