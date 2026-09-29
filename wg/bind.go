package wg

import (
	"net"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
)

// wsBind is the conn.Bind a websocket tunnel's WireGuard device sends and
// receives through: frames go straight onto the gateway websocket (with the
// 4-byte length prefix wswg expects) instead of through a loopback UDP
// socket. There's no port and no address; every packet goes to, and comes
// from, the one gateway the link is connected to.
//
// A bind belongs to one device. The link outlives it: token-mode tunnels
// replace their device (and so their bind) when a reconnect re-provisions
// the peer, while the websocket link carries on.
type wsBind struct {
	link *WsWgProxy

	mu     sync.Mutex
	open   bool
	closed chan struct{}
}

var _ conn.Bind = (*wsBind)(nil)

func newWsBind(link *WsWgProxy) *wsBind {
	return &wsBind{link: link}
}

// Open starts delivering the link's incoming frames to the device. port is
// meaningless for a websocket and is echoed back unchanged.
func (b *wsBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.open {
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	b.open = true
	b.closed = make(chan struct{})
	closed := b.closed

	recv := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		return b.link.receive(closed, packets, sizes, eps)
	}

	return []conn.ReceiveFunc{recv}, port, nil
}

// Close stops delivery: receive functions handed out by Open return
// net.ErrClosed. The link itself stays up. Safe to call more than once.
func (b *wsBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.open {
		close(b.closed)
		b.open = false
	}

	return nil
}

func (b *wsBind) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return !b.open
}

// Send writes each packet to the gateway as its own length-prefixed frame.
func (b *wsBind) Send(bufs [][]byte, _ conn.Endpoint) error {
	if b.isClosed() {
		return net.ErrClosed
	}

	for _, pkt := range bufs {
		if err := b.link.writePacket(pkt); err != nil {
			return err
		}
	}

	return nil
}

// ParseEndpoint accepts anything: there's only one place packets can go.
func (b *wsBind) ParseEndpoint(string) (conn.Endpoint, error) {
	return wsEndpoint{}, nil
}

// SetMark is a no-op; there's no socket to mark.
func (b *wsBind) SetMark(uint32) error { return nil }

// BatchSize is 1: each frame is read and written on its own.
func (b *wsBind) BatchSize() int { return 1 }

// wsEndpointAddr stands in for the gateway's address wherever wireguard-go
// wants one (rate limiting, cookie MACs, logs). It's never dialed.
var wsEndpointAddr = netip.MustParseAddr("2001:db8::7761:6d67") // "wswg"

// wsEndpoint is the single peer address a websocket bind knows: the
// gateway at the other end of the link.
type wsEndpoint struct{}

func (wsEndpoint) ClearSrc()           {}
func (wsEndpoint) SrcToString() string { return "" }
func (wsEndpoint) DstToString() string { return "wswg" }
func (wsEndpoint) DstToBytes() []byte  { return []byte("wswg") }
func (wsEndpoint) DstIP() netip.Addr   { return wsEndpointAddr }
func (wsEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }
