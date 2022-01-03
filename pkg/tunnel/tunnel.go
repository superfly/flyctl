// Package tunnel implemnets WireGuard tunneling.
package tunnel

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"

	"golang.zx2c4.com/go118/netip"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

type Config struct {
	PrivateKey device.NoisePrivateKey
	PublicKey  device.NoisePublicKey

	Local    netip.Prefix
	Remote   netip.Prefix
	DNS      netip.Addr
	Endpoint netip.AddrPort

	MTU       int
	KeepAlive int
}

func (cfg *Config) mtu() (mtu int) {
	if mtu = cfg.MTU; mtu < 1 {
		mtu = device.DefaultMTU
	}

	return
}

func (cfg *Config) keepAlive() (ka int) {
	if ka = cfg.KeepAlive; ka < 0 {
		ka = 5
	}

	return
}

func New(cfg Config) (*Tunnel, error) {
	tunDev, gNet, err := netstack.CreateNetTUN([]netip.Addr{cfg.Local.Addr()}, []netip.Addr{cfg.DNS}, cfg.mtu())
	if err != nil {
		return nil, err
	}

	wgDev := device.NewDevice(tunDev, device.NewLogger(device.LogLevelDebug, "(fly-ssh) "))

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "private_key=%x\n", cfg.PublicKey[:])
	fmt.Fprintf(&buf, "public_key=%x\n", cfg.PublicKey[:])
	fmt.Fprintf(&buf, "endpoint=%s\n", cfg.Endpoint)
	fmt.Fprintf(&buf, "allowed_ip=%s\n", cfg.Remote)
	fmt.Fprintf(&buf, "persistent_keepalive_interval=%d\n", cfg.keepAlive())

	if err := wgDev.IpcSetOperation(bufio.NewReader(&buf)); err != nil {
		return nil, err
	}
	wgDev.Up()

	dns := netip.AddrPortFrom(cfg.DNS, 53).String()

	return &Tunnel{
		dev: wgDev,
		tun: tunDev,
		net: gNet,
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return gNet.DialContext(ctx, "tcp", dns)
			},
		},
	}, nil
}

type Tunnel struct {
	dev      *device.Device
	tun      tun.Device
	net      *netstack.Net
	resolver *net.Resolver

	closeOnce sync.Once
}

func (t *Tunnel) Resolver() *net.Resolver {
	return t.resolver
}

func (t *Tunnel) Close() error {
	t.closeOnce.Do(func() {
		t.dev.Close()

		t.dev, t.net, t.tun = nil, nil, nil
	})

	return nil
}

func (t *Tunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.net.DialContext(ctx, network, addr)
}
