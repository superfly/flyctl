package wg

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/websocket"
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

func write(w io.Writer, buf []byte) error {
	var lbuf [4]byte
	binary.BigEndian.PutUint32(lbuf[:], uint32(len(buf)))
	if _, err := w.Write(lbuf[:]); err != nil {
		return err
	}

	if len(buf) == 0 {
		return nil
	}

	_, err := w.Write(buf)
	return err
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

func (wswg *WsWgProxy) Connect(endpoint string) error {
	rurl := fmt.Sprintf("wss://%s:443/", endpoint)

	log.Printf("(re-)connecting to %s", rurl)

	conf, _ := websocket.NewConfig(rurl, rurl)
	conf.TlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	// oh well, if it'll end horror
	ws, err := websocket.DialConfig(conf)
	if err != nil {
		return fmt.Errorf("websocket: %w", err)
	}

	var magic [4]byte
	binary.BigEndian.PutUint32(magic[:], 0x2FACED77)

	if _, err = ws.Write(magic[:]); err != nil {
		return fmt.Errorf("write websocket magic: %w", err)
	}

	if wswg.wsConn != nil {
		wswg.wsConn.Close()
	}
	wswg.wsConn = ws

	return nil
}

func isTimeout(e error) bool {
	if err, ok := e.(net.Error); ok && err.Timeout() {
		return true
	}

	return false
}

func (wswg *WsWgProxy) wsWrite(c net.Conn, b []byte) error {
	wswg.wrlock.Lock()
	defer wswg.wrlock.Unlock()

	c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return write(c, b)
}

func (wswg *WsWgProxy) ws2wg(ctx context.Context) {
	pbuf := make([]byte, 2000)

	for ctx.Err() == nil {
		wswg.lock.RLock()
		c := wswg.wsConn
		wswg.lock.RUnlock()

		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		pkt, err := read(c, pbuf)
		if err != nil {
			if isTimeout(err) {
				continue
			}

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
	buf := make([]byte, 2000)

	for ctx.Err() == nil {
		wswg.plugConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, a, err := wswg.plugConn.ReadFrom(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}

			// resetting won't do anything here
			log.Printf("error reading from udp plugboard: %s", err)
		}

		wswg.lock.Lock()
		wswg.lastPlugAddr = a
		c := wswg.wsConn
		wswg.lock.Unlock()

		if err = wswg.wsWrite(c, buf[:n]); err != nil {
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

	if err = wswg.Connect(endpoint); err != nil {
		return 0, err
	}

	go func() {
		defer wswg.wsConn.Close()
		defer wswg.plugConn.Close()

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGUSR1)

		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()

		reconnectAt := time.Time{}

		for {
			select {
			case <-tick.C:
				if !reconnectAt.IsZero() && reconnectAt.Before(time.Now()) {
					wswg.lock.Lock()
					wswg.Connect(endpoint)
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

		for ctx.Err() == nil {
			time.Sleep(1 * time.Second)

			if wswg.lastIo() > (1 * time.Second) {
				wswg.lock.RLock()
				c := wswg.wsConn
				wswg.lock.RUnlock()

				if err := wswg.wsWrite(c, nil); err != nil {
					wswg.resetConn(c, err)
				}
			}
		}
	}()

	return port, nil
}
