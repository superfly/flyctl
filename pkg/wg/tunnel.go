package wg

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/websocket"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

type Tunnel struct {
	dev    *device.Device
	tun    tun.Device
	net    *netstack.Net
	dnsIP  net.IP
	State  *WireGuardState
	Config *Config

	resolv *net.Resolver
}

func udpPlugboard(c net.Conn, ctx context.Context) (int, error) {
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

	doRead := func(c net.Conn, b []byte) bool {
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err := c.Read(b)
		if err != nil {
			log.Printf("read: %s", err)
			return false
		}
		return false
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
				log.Printf("read udp: %s", err)
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
		pbuf := make([]byte, 2000)

		for {
			if ctx.Err() != nil {
				return
			}

			var lbuf [4]byte
			if !doRead(c, lbuf[:]) {
				// we're broken here, kill the whole thing
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

func Connect(state *WireGuardState) (*Tunnel, error) {
	cfg := state.TunnelConfig()
	fmt.Println("wg connect", cfg.DNS, cfg.Endpoint, cfg.LocalNetwork.IP, cfg.RemoteNetwork.IP)
	localIPs := []net.IP{cfg.LocalNetwork.IP}
	dnsIP := cfg.DNS

	mtu := cfg.MTU
	if mtu == 0 {
		mtu = device.DefaultMTU
	}

	tunDev, gNet, err := netstack.CreateNetTUN(localIPs, []net.IP{dnsIP}, mtu)
	if err != nil {
		return nil, err
	}

	endpointHost, endpointPort, err := net.SplitHostPort(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	endpointIPs, err := net.LookupIP(endpointHost)
	if err != nil {
		return nil, err
	}

	endpointIP := endpointIPs[rand.Intn(len(endpointIPs))]
	endpointAddr := net.JoinHostPort(endpointIP.String(), endpointPort)

	wgDev := device.NewDevice(tunDev, device.NewLogger(cfg.LogLevel, "(fly-ssh) "))

	rurl, _ := url.Parse(fmt.Sprintf("ws://%s:443/", endpointIP.String()))
	lurl, _ := url.Parse("http://localhost")

	// oh well, if it'll end horror
	ws, err := websocket.DialConfig(&websocket.Config{
		TlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		Location: rurl,
		Origin:   lurl,
	})
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}

	plugPort, err := udpPlugboard(ws, context.Background())
	if err != nil {
		return nil, fmt.Errorf("plugboard: %w", err)
	}

	endpointAddr = fmt.Sprintf("127.0.0.1:%d", plugPort)

	wgConf := bytes.NewBuffer(nil)
	fmt.Fprintf(wgConf, "private_key=%s\n", cfg.LocalPrivateKey.ToHex())
	fmt.Fprintf(wgConf, "public_key=%s\n", cfg.RemotePublicKey.ToHex())
	fmt.Fprintf(wgConf, "endpoint=%s\n", endpointAddr)
	fmt.Fprintf(wgConf, "allowed_ip=%s\n", cfg.RemoteNetwork)
	fmt.Fprintf(wgConf, "persistent_keepalive_interval=%d\n", cfg.KeepAlive)

	if err := wgDev.IpcSetOperation(bufio.NewReader(wgConf)); err != nil {
		return nil, err
	}
	wgDev.Up()

	return &Tunnel{
		dev:    wgDev,
		tun:    tunDev,
		net:    gNet,
		dnsIP:  dnsIP,
		Config: cfg,
		State:  state,

		resolv: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				fmt.Println("resolver.Dial", network, address)
				return gNet.DialContext(ctx, "tcp", net.JoinHostPort(dnsIP.String(), "53"))
			},
		},
	}, nil
}

func (t *Tunnel) Close() error {
	if t.dev != nil {
		t.dev.Close()
	}

	t.dev, t.net, t.tun = nil, nil, nil
	return nil
}

func (t *Tunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.net.DialContext(ctx, network, addr)
}

func (t *Tunnel) Resolver() *net.Resolver {
	return t.resolv
}

func (t *Tunnel) queryDNS(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	client := dns.Client{
		Net: "tcp",
		Dialer: &net.Dialer{
			Resolver: t.resolv,
		},
	}

	c, err := t.DialContext(ctx, "tcp", net.JoinHostPort(t.dnsIP.String(), "53"))
	if err != nil {
		return nil, err
	}
	defer c.Close()

	conn := &dns.Conn{Conn: c}
	defer conn.Close()

	r, _, err := client.ExchangeWithConn(msg, conn)
	return r, err
}

func (t *Tunnel) LookupTXT(ctx context.Context, name string) ([]string, error) {
	m := &dns.Msg{}
	m.SetQuestion(dns.Fqdn(name), dns.TypeTXT)

	r, err := t.queryDNS(ctx, m)
	if err != nil {
		return nil, err
	}

	results := []string{}

	for _, a := range r.Answer {
		txtRecord := a.(*dns.TXT)
		results = append(results, txtRecord.Txt...)
	}

	return results, nil
}

func (t *Tunnel) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	m := &dns.Msg{}
	m.SetQuestion(dns.Fqdn(name), dns.TypeAAAA)

	r, err := t.queryDNS(ctx, m)
	if err != nil {
		return nil, err
	}

	results := []net.IP{}

	for _, a := range r.Answer {
		aaaaRecord := a.(*dns.AAAA)
		results = append(results, aaaaRecord.AAAA)
	}

	return results, nil
}
