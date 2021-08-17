// +build windows

package agent

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/superfly/flyctl/api"
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

func (c *noAgentClientProvider) Ping(ctx context.Context) (int, error) {
	return 0, nil
}

func (c *noAgentClientProvider) Establish(ctx context.Context, slug string) error {
	if c.Client == nil {
		return fmt.Errorf("no client set for stub agent")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.tunnels[slug]; ok {
		return nil
	}

	org, err := findOrganization(c.Client, slug)
	if err != nil {
		return err
	}

	tunnel, err := buildTunnel(c.Client, org)
	if err != nil {
		return err
	}

	c.tunnels[org.Slug] = tunnel
	return nil
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
	if err := c.Establish(ctx, o.Slug); err != nil {
		return nil, fmt.Errorf("dial: can't establish tunel: %s", err)
	}

	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return nil, fmt.Errorf("dial: can't build tunnel: %s", err)
	}

	return &noAgentDialer{
		Org:    o,
		tunnel: tunnel,
	}, nil
}

type noAgentDialer struct {
	Org    *api.Organization
	tunnel *wg.Tunnel
}

func (d *noAgentDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.tunnel.DialContext(ctx, network, addr)
}
