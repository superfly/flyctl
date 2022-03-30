package wg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"

	"github.com/miekg/dns"
	"golang.zx2c4.com/go118/netip"
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

	wscancel func()
	resolv   *net.Resolver
}

func Connect(ctx context.Context, state *WireGuardState) (*Tunnel, error) {
	return doConnect(ctx, state, false)
}

func doConnect(ctx context.Context, state *WireGuardState, wswg bool) (*Tunnel, error) {
	cfg := state.TunnelConfig()
	fmt.Println("wg connect", cfg.DNS, cfg.Endpoint, cfg.LocalNetwork.IP, cfg.RemoteNetwork.IP)
	localIPs := []netip.Addr{netip.AddrFromSlice(cfg.LocalNetwork.IP)}
	dnsIP := netip.AddrFromSlice(cfg.DNS)

	mtu := cfg.MTU
	if mtu == 0 {
		mtu = device.DefaultMTU
	}

	tunDev, gNet, err := netstack.CreateNetTUN(localIPs, []netip.Addr{dnsIP}, mtu)
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

	if wswg {
		port, err := websocketConnect(ctx, endpointHost)
		if err != nil {
			return nil, err
		}

		endpointAddr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	wgDev := device.NewDevice(tunDev, device.NewLogger(cfg.LogLevel, "(fly-ssh) "))

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
		dnsIP:  cfg.DNS,
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
	if t.wscancel != nil {
		t.wscancel()
		t.wscancel = nil
	}

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

func (t *Tunnel) LookupTXT(ctx context.Context, name string) ([]string, error) {
	var m dns.Msg
	_ = m.SetQuestion(dns.Fqdn(name), dns.TypeTXT)

	r, err := t.queryDNS(ctx, &m)
	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(r.Answer))

	for _, a := range r.Answer {
		txt := a.(*dns.TXT)

		results = append(results, txt.Txt...)
	}

	return results, nil
}

func (t *Tunnel) ListenPing() (*netstack.PingConn, error) {
	laddr := netip.AddrFromSlice(t.Config.LocalNetwork.IP)
	raddr := netip.IPv6Unspecified()

	conn, err := t.net.DialPingAddr(laddr, raddr)
	if err != nil {
		return nil, fmt.Errorf("ping listener: %w", err)
	}

	return conn, nil
}

func (t *Tunnel) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	var m dns.Msg
	_ = m.SetQuestion(dns.Fqdn(name), dns.TypeAAAA)

	r, err := t.queryDNS(ctx, &m)
	if err != nil {
		return nil, err
	}

	results := make([]net.IP, 0, len(r.Answer))

	for _, a := range r.Answer {
		ip := a.(*dns.AAAA).AAAA
		results = append(results, ip)
	}

	return results, nil
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
