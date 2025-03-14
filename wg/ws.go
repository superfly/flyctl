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
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
)

func ConnectWS(ctx context.Context, state *WireGuardState) (*Tunnel, error) {
	ctx, cancel := context.WithCancel(ctx)

	t, err := doConnect(ctx, state, true)
	if err == nil {
		t.wscancel = cancel
	} else {
		cancel()
	}

	return t, err
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

func (wswg *WsWgProxy) resetConn(c net.Conn, err error) {
	wswg.lock.RLock()
	cur := wswg.wsConn
	wswg.lock.RUnlock()

	if cur != c {
		return
	}

	wswg.limit.Wait(context.Background())

	log.Printf("resetting connection due to error: %s", err)
	wswg.reset <- true
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

func (wswg *WsWgProxy) Connect(ctx context.Context, endpoint string) error {
	rurl := fmt.Sprintf("wss://%s:443/", endpoint)

	log.Printf("(re-)connecting to %s", rurl)

	ws, _, err := websocket.Dial(ctx, rurl, &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				// It's fine. The traffic inside the tunnel is already encrypted by WG
				TLSClientConfig: &tls.Config{ // skipcq: GO-S1020
					InsecureSkipVerify: true, // skipcq: GSC-G402
				},
			},
		},
		HTTPHeader: http.Header{
			"Origin": []string{rurl},
		},
	})
	if err != nil {
		return fmt.Errorf("websocket: %w", err)
	}

	wsConn := websocket.NetConn(ctx, ws, websocket.MessageText)

	var magic [4]byte
	binary.BigEndian.PutUint32(magic[:], 0x2FACED77)

	if _, err = wsConn.Write(magic[:]); err != nil {
		return fmt.Errorf("write websocket magic: %w", err)
	}

	if wswg.wsConn != nil {
		_ = wswg.wsConn.Close()
	}
	wswg.wsConn = wsConn

	return nil
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
			wswg.resetConn(c, err)
		}

		wswg.touch()

		wswg.lock.RLock()
		addr := wswg.lastPlugAddr
		wswg.lock.RUnlock()

		if _, err = wswg.plugConn.WriteTo(pkt, addr); err != nil {
			wswg.resetConn(c, err)
		}
	}
}

func (wswg *WsWgProxy) wg2ws(ctx context.Context) {
	const (
		bufLen = 0xffff
		mtu    = 2000 // really closer to 1500, but going big avoids needing to be precise
	)

	var buf [bufLen]byte

	for ctx.Err() == nil {
		if err := wswg.plugConn.SetReadDeadline(time.Time{}); err != nil {
			log.Printf("error resetting read deadline: %s", err)
			return
		}

		msgLen := 0

		n, a, err := wswg.plugConn.ReadFrom(buf[msgLen+4 : msgLen+4+mtu])
		if err != nil {
			log.Printf("error reading from udp plugboard: %s", err)
			return
		}
		if n == mtu {
			log.Printf("oversized udp packet from %s", a)
			return
		}
		binary.BigEndian.PutUint32(buf[msgLen:], uint32(n))
		msgLen += n + 4

		wswg.lock.Lock()
		wswg.lastPlugAddr = a
		wswg.lock.Unlock()

		// read as much as we can in the next 10ms to minimize ws messages
		if err := wswg.plugConn.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
			log.Printf("error setting read deadline: %s", err)
			return
		}

		for msgLen < bufLen-mtu-4 {
			n, a, err = wswg.plugConn.ReadFrom(buf[msgLen+4 : msgLen+4+mtu])
			if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				log.Printf("error reading from udp plugboard: %s", err)
				return
			}
			if n == mtu {
				log.Printf("oversized udp packet from %s", a)
				return
			}
			if n == 0 {
				break
			}

			binary.BigEndian.PutUint32(buf[msgLen:], uint32(n))
			msgLen += n + 4
		}

		wswg.lock.RLock()
		c := wswg.wsConn
		wswg.lock.RUnlock()

		if err = wswg.wsWrite(c, buf[:msgLen]); err != nil {
			wswg.resetConn(c, err)
		}

		wswg.touch()
	}
}

func websocketConnect(ctx context.Context, endpoint string) (int, error) {
	wswg, err := NewWsWgProxy()
	if err != nil {
		return 0, err
	}

	port, err := wswg.Port()
	if err != nil {
		return 0, err
	}

	if err = wswg.Connect(ctx, endpoint); err != nil {
		return 0, err
	}

	go func() {
		defer wswg.wsConn.Close()   // skipcq: GO-S2307
		defer wswg.plugConn.Close() // skipcq: GO-S2307

		c := make(chan os.Signal, 1)
		signalChannel(c)

		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()

		reconnectAt := time.Time{}

		for {
			select {
			case <-tick.C:
				if !reconnectAt.IsZero() && reconnectAt.Before(time.Now()) {
					wswg.lock.Lock()
					wswg.Connect(ctx, endpoint)
					wswg.lock.Unlock()

					reconnectAt = time.Time{}
				}

			case <-ctx.Done():
				return

			case <-c:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}

			case <-wswg.reset:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}
			}
		}
	}()

	go func() {
		go wswg.ws2wg(ctx)
		go wswg.wg2ws(ctx)

		emptyMessage := binary.BigEndian.AppendUint32(nil, 0)

		for ctx.Err() == nil {
			time.Sleep(1 * time.Second)

			if wswg.lastIo() > (1 * time.Second) {
				wswg.lock.RLock()
				c := wswg.wsConn
				wswg.lock.RUnlock()

				if err := wswg.wsWrite(c, emptyMessage); err != nil {
					wswg.resetConn(c, err)
				}
			}
		}
	}()

	return port, nil
}
