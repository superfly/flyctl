// +build linux

package agent

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/wg"
)

type Client struct {
	Client  *api.Client
	tunnels map[string]*wg.Tunnel
	lock    sync.Mutex
}

func (c *Client) tunnelFor(slug string) (*wg.Tunnel, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	tunnel, ok := c.tunnels[slug]
	if !ok {
		return nil, fmt.Errorf("no tunnel for %s established", slug)
	}

	return tunnel, nil
}

func NewClient(path string) (*Client, error) {
	return &Client{
		tunnels: map[string]*wg.Tunnel{},
	}, nil
}

func DefaultClient() (*Client, error) {
	return NewClient("")
}

func (c *Client) Kill() error {
	return nil
}

func (c *Client) Ping() (int, error) {
	return 0, nil
}

func (c *Client) Establish(slug string) error {
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

func (c *Client) Probe(o *api.Organization) error {
	tunnel, err := c.tunnelFor(o.Slug)
	if err != nil {
		return fmt.Errorf("can't build tunnel: %s", err)
	}

	if err := probeTunnel(tunnel); err != nil {
		return err
	}

	return nil
}

func (c *Client) Instances(o *api.Organization, app string) (*Instances, error) {
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

type Dialer struct {
	Org     *api.Organization
	Timeout time.Duration
}

func (c *Client) Dialer(o *api.Organization) (*Dialer, error) {
	return nil, fmt.Errorf("not implemented yet")
}

func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return nil, fmt.Errorf("not implemented yet")
}
