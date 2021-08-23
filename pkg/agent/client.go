package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
)

/// Establish starts the daemon if necessary and returns a client
func Establish(ctx context.Context, apiClient *api.Client) (*Client, error) {
	if err := wireguard.PruneInvalidPeers(apiClient); err != nil {
		return nil, err
	}

	c, err := DefaultClient(apiClient)
	if err == nil {
		resp, err := c.Ping(ctx)
		if err == nil {
			if buildinfo.Version().EQ(resp.Version) {
				return c, nil
			}

			msg := fmt.Sprintf("flyctl version %s does not match agent version %s", buildinfo.Version(), resp.Version)

			if !resp.Background {
				terminal.Warn(msg)
				return c, nil
			}

			terminal.Debug(msg)
			terminal.Debug("stopping agent")
			if err := c.Kill(ctx); err != nil {
				terminal.Warn(msg)
				return nil, errors.Wrap(err, "kill failed")
			}
			// this is gross, but we need to wait for the agent to exit
			time.Sleep(1 * time.Second)
		}
	}

	return StartDaemon(ctx, apiClient, os.Args[0])
}

func NewClient(path string, apiClient *api.Client) (*Client, error) {
	provider, err := newClientProvider(path, apiClient)
	if err != nil {
		return nil, err
	}

	return &Client{provider: provider}, nil
}

func DefaultClient(apiClient *api.Client) (*Client, error) {
	path := fmt.Sprintf("%s/.fly/fly-agent.sock", os.Getenv("HOME"))
	return NewClient(path, apiClient)
}

type Client struct {
	provider clientProvider
}

func (c *Client) Kill(ctx context.Context) error {
	if err := c.provider.Kill(ctx); err != nil {
		return errors.Wrap(err, "kill failed")
	}
	return nil
}

type PingResponse struct {
	PID        int
	Version    semver.Version
	Background bool
}

func (c *Client) Ping(ctx context.Context) (PingResponse, error) {
	n, err := c.provider.Ping(ctx)
	if err != nil {
		return n, errors.Wrap(err, "ping failed")
	}
	return n, nil
}

type EstablishResponse struct {
	WireGuardState *wg.WireGuardState
	TunnelConfig   *wg.Config
}

func (c *Client) Establish(ctx context.Context, slug string) (*EstablishResponse, error) {
	resp, err := c.provider.Establish(ctx, slug)
	if err != nil {
		return nil, errors.Wrap(err, "establish failed")
	}
	return resp, nil
}

func (c *Client) WaitForTunnel(ctx context.Context, o *api.Organization) error {
	errCh := make(chan error, 1)

	go func() {
		for {
			err := c.Probe(ctx, o)
			if err != nil && IsTunnelError(err) {
				continue
			}

			errCh <- err
			break
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *Client) WaitForHost(ctx context.Context, o *api.Organization, host string) error {
	errCh := make(chan error, 1)

	go func() {
		for {
			_, err := c.Resolve(ctx, o, host)
			if err != nil && (IsHostNotFoundError(err) || IsTunnelError(err)) {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			errCh <- err
			break
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *Client) Probe(ctx context.Context, o *api.Organization) error {
	if err := c.provider.Probe(ctx, o); err != nil {
		err = mapResolveError(err, o.Slug, "")
		return errors.Wrap(err, "probe failed")
	}
	return nil
}

func (c *Client) Resolve(ctx context.Context, o *api.Organization, host string) (string, error) {
	addr, err := c.provider.Resolve(ctx, o, host)
	if err != nil {
		err = mapResolveError(err, o.Slug, host)
		return "", errors.Wrap(err, "resolve failed")
	}
	return addr, nil
}

func (c *Client) Instances(ctx context.Context, o *api.Organization, app string) (*Instances, error) {
	instances, err := c.provider.Instances(ctx, o, app)
	if err != nil {
		return nil, errors.Wrap(err, "list instances failed")
	}
	return instances, nil
}

func (c *Client) Proxy(ctx context.Context, addr, app string) error {
	err := c.provider.Proxy(ctx, addr, app)
	if err != nil {
		return errors.Wrap(err, "starting proxy failed")
	}
	return nil
}

func (c *Client) Unproxy(ctx context.Context) error {
	err := c.provider.Unproxy(ctx)
	if err != nil {
		return errors.Wrap(err, "stopping proxy failed")
	}
	return nil
}

func (c *Client) Dialer(ctx context.Context, o *api.Organization) (Dialer, error) {
	dialer, err := c.provider.Dialer(ctx, o)
	if err != nil {
		err = mapResolveError(err, o.Slug, "")
		return nil, errors.Wrap(err, "error fetching dialer")
	}
	return dialer, nil
}

// clientProvider is an interface for client functions backed by either the agent or in-process on Windows
type clientProvider interface {
	Dialer(ctx context.Context, o *api.Organization) (Dialer, error)
	Establish(ctx context.Context, slug string) (*EstablishResponse, error)
	Instances(ctx context.Context, o *api.Organization, app string) (*Instances, error)
	Kill(ctx context.Context) error
	Ping(ctx context.Context) (PingResponse, error)
	Probe(ctx context.Context, o *api.Organization) error
	Resolve(ctx context.Context, o *api.Organization, name string) (string, error)
	Proxy(ctx context.Context, addr, app string) error
	Unproxy(ctx context.Context) error
}

type Dialer interface {
	State() *wg.WireGuardState
	Config() *wg.Config
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func IsIPv6(addr string) bool {
	addr = strings.Trim(addr, "[]")
	ip := net.ParseIP(addr)
	return ip != nil && ip.To16() != nil
}
