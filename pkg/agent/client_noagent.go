// +build windows

package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/pkg/wg"
)

func newClientProvider(path string, api *api.Client) (clientProvider, error) {
	return &noAgentClientProvider{
		tunnels: map[string]*wg.Tunnel{},
		Client:  api,
	}, nil
}

type noAgentClientProvider struct {
	Client  *api.Client
	tunnels map[string]*wg.Tunnel
	lock    sync.Mutex
}

func (c *noAgentClientProvider) tunnelFor(slug string) (*wg.Tunnel, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	tunnel, ok := c.tunnels[slug]
	if !ok {
		return nil, fmt.Errorf("no tunnel for %s established", slug)
	}

	return tunnel, nil
}

func (c *noAgentClientProvider) Kill(ctx context.Context) error {
	return nil
}

func (c *noAgentClientProvider) Ping(ctx context.Context) (PingResponse, error) {
	resp := PingResponse{
		Version: buildinfo.Version(),
		PID:     os.Getpid(),
	}
	return resp, nil
}

func (c *noAgentClientProvider) Establish(ctx context.Context, slug string) (*EstablishResponse, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("no client set for stub agent")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	tunnel, ok := c.tunnels[slug]
	if !ok {
		org, err := findOrganization(c.Client, slug)
		if err != nil {
			return nil, err
		}

		tunnel, err = buildTunnel(c.Client, org)
		if err != nil {
			return nil, err
		}
		c.tunnels[slug] = tunnel
	}

	resp := &EstablishResponse{
		WireGuardState: tunnel.State,
		TunnelConfig:   tunnel.Config,
	}

	return resp, nil
}

func (c *noAgentClientProvider) Probe(ctx context.Context, o *api.Organization) error {
	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return fmt.Errorf("probe: can't build tunnel: %s", err)
	}

	if err := probeTunnel(ctx, tunnel); err != nil {
		return err
	}

	return nil
}

func (c *noAgentClientProvider) Resolve(ctx context.Context, o *api.Organization, host string) (string, error) {
	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return "", fmt.Errorf("probe: can't build tunnel: %s", err)
	}

	return resolve(tunnel, host)
}

func (c *noAgentClientProvider) Instances(ctx context.Context, o *api.Organization, app string) (*Instances, error) {
	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel: %s", err)
	}

	ret, err := fetchInstances(tunnel, app)
	if err != nil {
		return nil, err
	}

	if len(ret.Addresses) == 0 {
		return nil, fmt.Errorf("no running hosts for %s found", app)
	}

	return ret, nil
}

func (c *noAgentClientProvider) Dialer(ctx context.Context, o *api.Organization) (Dialer, error) {
	resp, err := c.Establish(ctx, o.Slug)
	if err != nil {
		return nil, fmt.Errorf("dial: can't establish tunel: %s", err)
	}

	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return nil, fmt.Errorf("dial: can't build tunnel: %s", err)
	}

	return &noAgentDialer{
		Org:    o,
		tunnel: tunnel,
		state:  resp.WireGuardState,
		config: resp.TunnelConfig,
	}, nil
}

type noAgentDialer struct {
	Org    *api.Organization
	tunnel *wg.Tunnel
	state  *wg.WireGuardState
	config *wg.Config
}

func (d *noAgentDialer) State() *wg.WireGuardState {
	return d.state
}

func (d *noAgentDialer) Config() *wg.Config {
	return d.config
}

func (d *noAgentDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.tunnel.DialContext(ctx, network, addr)
}
