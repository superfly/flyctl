package wg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
)

// WsBinarySubprotocol is the websocket subprotocol a client offers to get
// WireGuard frames back as binary websocket messages. Gateways that
// support it echo it; the frames themselves are unchanged. Browsers
// enforce UTF-8 on text frames, so the browser client requires it.
const WsBinarySubprotocol = "wswg-binary"

// maxFrame bounds a single relayed WireGuard frame. The device's MTU is
// well under this; anything bigger is a framing error.
const maxFrame = device.MaxMessageSize

func ConnectWS(ctx context.Context, state *WireGuardState) (*Tunnel, error) {
	return doConnect(ctx, state, true)
}

var errFrameTooLarge = errors.New("wswg frame exceeds maximum size")

// readFrame reads one length-prefixed frame into buf and returns its
// length. A zero-length frame is a keepalive.
func readFrame(r io.Reader, buf []byte) (int, error) {
	var lbuf [4]byte
	if _, err := io.ReadFull(r, lbuf[:]); err != nil {
		return 0, err
	}

	plen := binary.BigEndian.Uint32(lbuf[:])
	if plen > uint32(len(buf)) {
		return 0, fmt.Errorf("%w: %d bytes", errFrameTooLarge, plen)
	}

	if _, err := io.ReadFull(r, buf[:plen]); err != nil {
		return 0, err
	}

	return int(plen), nil
}

// WsWgProxy is the websocket link to a wswg gateway: it dials, (in token
// mode) authenticates, reconnects when the connection drops, keeps the
// session alive, and relays WireGuard frames for the device behind a
// wsBind. The device sends through writePacket directly; incoming frames
// are read by one goroutine into a channel the bind's receive function
// drains, so a bind can be closed without waiting on a blocked read.
type WsWgProxy struct {
	// connMu guards the connection itself and the fields that follow it.
	// It's held only briefly, never across a dial, so the packet paths can
	// take it while the reconnect loop is busy.
	connMu sync.RWMutex
	wsConn net.Conn
	// swapped is closed (and replaced) whenever wsConn is replaced, to wake
	// the reader parked on a dead connection.
	swapped chan struct{}
	// dead is the connection a reset has already been requested for, so
	// every failed read and write on it doesn't request another.
	dead  net.Conn
	atime time.Time

	// wrlock serializes writes to the websocket. wbuf is the write
	// scratch space it protects.
	wrlock sync.Mutex
	wbuf   []byte

	// incoming carries frames from the reader to the bind. It's bounded;
	// frames nobody is receiving are dropped, like UDP would.
	incoming chan []byte
	pool     sync.Pool
	lifetime context.Context

	reset chan struct{}

	// lock guards the token-mode session state below.
	lock sync.RWMutex

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

// websocketURL builds the gateway's wswg URL; endpoint is a hostname, or
// host:port for gateways not on 443, or a full ws:// or wss:// URL for
// local testing.
func websocketURL(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}

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

func NewWsWgProxy(lifetimeCtx context.Context) *WsWgProxy {
	return &WsWgProxy{
		atime:    time.Now(),
		swapped:  make(chan struct{}),
		wbuf:     make([]byte, 4+maxFrame),
		incoming: make(chan []byte, 64),
		pool:     sync.Pool{New: func() any { return make([]byte, maxFrame) }},
		lifetime: lifetimeCtx,
		reset:    make(chan struct{}, 1),
	}
}

// bind returns a fresh conn.Bind for a device to send and receive through
// this link.
func (wswg *WsWgProxy) bind() *wsBind {
	return newWsBind(wswg)
}

func (wswg *WsWgProxy) touch() {
	wswg.connMu.Lock()
	wswg.atime = time.Now()
	wswg.connMu.Unlock()
}

func (wswg *WsWgProxy) lastIo() time.Duration {
	wswg.connMu.RLock()
	s := time.Since(wswg.atime)
	wswg.connMu.RUnlock()

	return s
}

// current returns the live connection, the channel that closes when it's
// replaced, and whether it's already known to be dead.
func (wswg *WsWgProxy) current() (c net.Conn, swapped chan struct{}, dead bool) {
	wswg.connMu.RLock()
	defer wswg.connMu.RUnlock()

	return wswg.wsConn, wswg.swapped, wswg.wsConn == wswg.dead
}

// resetConn asks the reconnect loop for a new connection because c failed
// with err. Only the first failure of the live connection counts; later
// ones (and failures of a connection that's already been replaced) are
// ignored, and the reconnect loop paces the attempts.
func (wswg *WsWgProxy) resetConn(ctx context.Context, c net.Conn, err error) {
	if ctx.Err() != nil {
		return
	}

	wswg.connMu.Lock()
	if c != wswg.wsConn || c == wswg.dead {
		wswg.connMu.Unlock()

		return
	}
	wswg.dead = c
	wswg.connMu.Unlock()

	log.Printf("resetting connection due to error: %s", err)

	select {
	case wswg.reset <- struct{}{}:
	default:
	}
}

// swap installs c as the live connection and closes the old one.
func (wswg *WsWgProxy) swap(c net.Conn) {
	wswg.connMu.Lock()
	old := wswg.wsConn
	wswg.wsConn = c
	wswg.dead = nil
	close(wswg.swapped)
	wswg.swapped = make(chan struct{})
	wswg.connMu.Unlock()

	if old != nil {
		_ = old.Close()
	}
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
		prov, err := tokenExchange(wsConn, wswg.auth, token, wsAuthHeaders && token != "")
		if err != nil {
			_ = wsConn.Close()

			if permanentTokenFailure(err) && wswg.onFatal != nil {
				wswg.onFatal(err)
			}

			return fmt.Errorf("token exchange: %w", err)
		}

		if wswg.prov != nil && !wswg.prov.same(prov) {
			// Anycast: the reconnect landed on another edge, which has its
			// own key and allocated us a fresh address. The owner rebuilds
			// the device around it once the new connection is in place.
			log.Printf("gateway re-provisioned our peer (%s -> %s)", wswg.prov.PeerIP, prov.PeerIP)

			reprovisioned = prov
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

	wswg.swap(wsConn)

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

// close releases the proxy's connection; for a proxy that was never started.
func (wswg *WsWgProxy) close() {
	wswg.connMu.Lock()
	c := wswg.wsConn
	wswg.connMu.Unlock()

	if c != nil {
		_ = c.Close()
	}
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

// wsWrite writes one frame (already length-prefixed) to c.
func (wswg *WsWgProxy) wsWrite(c net.Conn, b []byte) error {
	wswg.wrlock.Lock()
	defer wswg.wrlock.Unlock()

	_, err := c.Write(b)

	return err
}

// writePacket sends one WireGuard packet to the gateway. A packet for a
// connection that's known to be dead is dropped, as a UDP send into the
// void would be; WireGuard retransmits what matters.
func (wswg *WsWgProxy) writePacket(pkt []byte) error {
	if len(pkt) > maxFrame {
		return errFrameTooLarge
	}

	c, _, dead := wswg.current()
	if c == nil || dead {
		return nil
	}

	wswg.wrlock.Lock()
	binary.BigEndian.PutUint32(wswg.wbuf, uint32(len(pkt)))
	n := copy(wswg.wbuf[4:], pkt)
	_, err := c.Write(wswg.wbuf[:4+n])
	wswg.wrlock.Unlock()

	if err != nil {
		wswg.resetConn(wswg.lifetime, c, err)

		return nil
	}

	wswg.touch()

	return nil
}

// receive hands the next incoming frame to a device, or reports
// net.ErrClosed once closed (the bind's) or the link's lifetime ends.
func (wswg *WsWgProxy) receive(closed <-chan struct{}, packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	select {
	case frame := <-wswg.incoming:
		n := copy(packets[0], frame)
		sizes[0] = n
		eps[0] = wsEndpoint{}
		wswg.pool.Put(frame[:cap(frame)]) // nolint:staticcheck

		return 1, nil
	case <-closed:
		return 0, net.ErrClosed
	case <-wswg.lifetime.Done():
		return 0, net.ErrClosed
	}
}

// readLoop reads frames off whichever connection is live and queues them
// for the bind, parking on a dead connection until the reconnect loop
// replaces it.
func (wswg *WsWgProxy) readLoop(ctx context.Context) {
	for ctx.Err() == nil {
		c, swapped, dead := wswg.current()
		if c == nil || dead {
			select {
			case <-swapped:
			case <-ctx.Done():
			}

			continue
		}

		buf := wswg.pool.Get().([]byte)

		n, err := readFrame(c, buf)
		if err != nil {
			wswg.pool.Put(buf) // nolint:staticcheck
			wswg.resetConn(ctx, c, err)

			continue
		}

		wswg.touch()

		if n == 0 {
			// keepalive
			wswg.pool.Put(buf) // nolint:staticcheck

			continue
		}

		select {
		case wswg.incoming <- buf[:n]:
		default:
			// no device is draining (it's being rebuilt) or it's swamped:
			// drop, as the network would
			wswg.pool.Put(buf) // nolint:staticcheck
		}
	}
}

// websocketConnect dials a legacy (magic-authenticated) gateway and starts
// the link; the tunnel's device is expected to follow through bind().
func websocketConnect(dialCtx, lifetimeCtx context.Context, endpoint string) (*WsWgProxy, error) {
	wswg, err := websocketConnectAuth(dialCtx, lifetimeCtx, endpoint, nil)
	if err != nil {
		return nil, err
	}

	wswg.start(lifetimeCtx, endpoint)

	return wswg, nil
}

// websocketConnectAuth dials the gateway once, with optional token
// authentication (a non-nil auth makes every (re-)connection perform the
// token exchange; wswg.prov holds the latest provision). Nothing runs yet:
// the caller inspects the proxy, installs callbacks, and then calls start.
func websocketConnectAuth(dialCtx, lifetimeCtx context.Context, endpoint string, auth *TokenAuth) (*WsWgProxy, error) {
	wswg := NewWsWgProxy(lifetimeCtx)
	wswg.auth = auth

	if err := wswg.Connect(dialCtx, lifetimeCtx, endpoint); err != nil {
		return nil, err
	}

	return wswg, nil
}

// start runs the reader, reconnect and keepalive loops until lifetimeCtx is
// canceled.
func (wswg *WsWgProxy) start(lifetimeCtx context.Context, endpoint string) {
	go wswg.readLoop(lifetimeCtx)

	go func() {
		defer wswg.close()

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
					reconnectAt = time.Time{}

					wswg.lock.Lock()
					wswg.refreshAt = time.Time{}
					if err := wswg.Connect(lifetimeCtx, lifetimeCtx, endpoint); err != nil {
						log.Printf("reconnect failed: %s", err)

						// After a failed refresh the old connection is still
						// fine; try again before the session expires. After a
						// dropped connection there's nothing to do but retry.
						if wswg.prov != nil {
							wswg.scheduleRefresh(wswg.prov)
						}
						if _, _, dead := wswg.current(); dead {
							reconnectAt = now.Add(reconnectInterval)
						}
					} else {
						// A token gateway drops the previous session once the
						// new one authenticates, which the reader reports as a
						// reset; that connection is already replaced.
						select {
						case <-wswg.reset:
						default:
						}
					}
					wswg.lock.Unlock()
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
		zeroLenMsg := make([]byte, 4)

		for lifetimeCtx.Err() == nil {
			time.Sleep(1 * time.Second)

			if wswg.lastIo() > (1 * time.Second) {
				c, _, dead := wswg.current()
				if c == nil || dead {
					continue
				}

				if err := wswg.wsWrite(c, zeroLenMsg); err != nil {
					wswg.resetConn(lifetimeCtx, c, err)
				}
			}
		}
	}()
}
