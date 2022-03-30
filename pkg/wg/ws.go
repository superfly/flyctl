package wg

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

func ConnectWS(ctx context.Context, state *WireGuardState) (*Tunnel, error) {
	return doConnect(ctx, state, true)
}

// this is gross, but, keep the rest of the WireGuard code in
// flyctl oblivious to the fact that we're potentially proxying
// it over tcp.
func udpPlugboard(ctx context.Context, c net.Conn) (int, error) {
	laddr := net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	}

	l, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}

	bindAddr := l.LocalAddr()
	udpBindAddr, ok := bindAddr.(*net.UDPAddr)
	if !ok {
		return 0, fmt.Errorf("plugboard: can't recover UDP port")
	}

	doWrite := func(b []byte) bool {
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := c.Write(b)
		if err != nil {
			log.Printf("write: %s", err)
			return false
		}
		return true
	}

	isTimeout := func(e error) bool {
		if err, ok := e.(net.Error); ok && err.Timeout() {
			return true
		}

		return false
	}

	doRead := func(c net.Conn, b []byte) bool {
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err := c.Read(b)
		if err != nil {
			if !isTimeout(err) {
				log.Printf("read: %s", err)
			}
			return false
		}
		return true
	}

	var (
		addr net.Addr
		lock sync.Mutex
	)

	readSide := func() {
		buf := make([]byte, 2000)

		for {
			if ctx.Err() != nil {
				return
			}

			l.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, a, err := l.ReadFrom(buf)
			if err != nil {
				if !isTimeout(err) {
					log.Printf("read udp: %s", err)
				}

				continue
			}

			lock.Lock()
			addr = a
			lock.Unlock()

			var lbuf [4]byte
			binary.BigEndian.PutUint32(lbuf[:], uint32(n))
			if !doWrite(lbuf[:]) {
				continue
			}

			if !doWrite(buf[:n]) {
				continue
			}
		}
	}

	writeSide := func() {
		defer c.Close()

		pbuf := make([]byte, 2000)

		for {
			if ctx.Err() != nil {
				return
			}

			var lbuf [4]byte
			if !doRead(c, lbuf[:]) {
				continue
			}

			plen := binary.BigEndian.Uint32(lbuf[:])
			if plen > 1500 {
				log.Printf("martian length: %d", plen)
				continue
			}

			if !doRead(c, pbuf[:plen]) {
				continue
			}

			lock.Lock()
			_, err := l.WriteTo(pbuf[:plen], addr)
			lock.Unlock()
			if err != nil {
				log.Printf("udp write: %s", err)
			}
		}
	}

	go readSide()
	go writeSide()

	return udpBindAddr.Port, nil
}

func websocketConnect(ctx context.Context, endpoint string) (int, error) {
	rurl := fmt.Sprintf("wss://%s:443/", endpoint)
	conf, _ := websocket.NewConfig(rurl, rurl)
	conf.TlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	// oh well, if it'll end horror
	ws, err := websocket.DialConfig(conf)
	if err != nil {
		return 0, fmt.Errorf("websocket: %w", err)
	}

	var magic [4]byte
	binary.BigEndian.PutUint32(magic[:], 0x2FACED77)

	if _, err = ws.Write(magic[:]); err != nil {
		return 0, fmt.Errorf("write websocket magic: %w", err)
	}

	plugPort, err := udpPlugboard(ctx, ws)
	if err != nil {
		return 0, fmt.Errorf("plugboard: %w", err)
	}

	return plugPort, nil
}
