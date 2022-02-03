package wg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"

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

	Resolver *Resolver
	resolv   *net.Resolver
}

func Connect(state *WireGuardState) (*Tunnel, error) {
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

	t := &Tunnel{
		dev:    wgDev,
		tun:    tunDev,
		net:    gNet,
		dnsIP:  cfg.DNS,
		Config: cfg,
		State:  state,

		resolv: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return gNet.DialContext(ctx, "tcp", net.JoinHostPort(dnsIP.String(), "53"))
			},
		},
	}

	resolver, err := NewResolver(t, cfg.DNS.String())
	if err != nil {
		return nil, err
	}
	t.Resolver = resolver

	return t, nil
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

func (t *Tunnel) ListenPing() (*netstack.PingConn, error) {
	laddr := netip.AddrFromSlice(t.Config.LocalNetwork.IP)
	raddr := netip.IPv6Unspecified()

	conn, err := t.net.DialPingAddr(laddr, raddr)
	if err != nil {
		return nil, fmt.Errorf("ping listener: %w", err)
	}

	return conn, nil
}

func (t *Tunnel) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return t.Resolver.LookupTXT(ctx, name)
}

func (t *Tunnel) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	return t.Resolver.LookupAAAA(ctx, name)
}

func (t *Tunnel) LookupHost(ctx context.Context, name string) ([]string, error) {
	return t.Resolver.LookupHost(ctx, name)
}
