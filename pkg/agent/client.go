package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/azazeal/pause"
	"github.com/blang/semver"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/pkg/agent/internal/proto"
	"github.com/superfly/flyctl/pkg/wg"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/wireguard"
)

// Establish starts the daemon, if necessary, and returns a client to it.
func Establish(ctx context.Context, apiClient *api.Client) (*Client, error) {
	if err := wireguard.PruneInvalidPeers(ctx, apiClient); err != nil {
		return nil, err
	}

	c, err := DefaultClient(ctx)
	if err != nil {
		return StartDaemon(ctx)
	}

	res, err := c.Ping(ctx)
	if err != nil {
		return nil, err
	}

	if buildinfo.Version().EQ(res.Version) {
		return c, nil
	}

	// TOOD: log this instead
	msg := fmt.Sprintf("flyctl version %s does not match agent version %s", buildinfo.Version(), res.Version)
	if logger := logger.MaybeFromContext(ctx); logger != nil {
		logger.Warn(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}

	if !res.Background {
		return c, nil
	}

	fmt.Fprintln(os.Stderr, "stopping agent ...")
	if err := c.Kill(ctx); err != nil {
		err = fmt.Errorf("failed stopping agent: %w", err)
		fmt.Fprintln(os.Stderr, err)

		return nil, err
	}

	// this is gross, but we need to wait for the agent to exit
	pause.For(ctx, time.Second)

	return nil, nil
}

func NewClient(ctx context.Context, network, addr string) (client *Client, err error) {
	client = &Client{
		network: network,
		address: addr,
	}

	if _, err = client.Ping(ctx); err != nil {
		client = nil
	}

	return
}

// TODO: deprecate
func DefaultClient(ctx context.Context) (*Client, error) {
	return NewClient(ctx, "unix", PathToSocket())
}

const (
	timeout = 2 * time.Second
	cycle   = time.Second / 10
)

type Client struct {
	network string
	address string
	dialer  net.Dialer
}

func (c *Client) dial() (conn net.Conn, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return c.dialContext(ctx)
}

func (c *Client) dialContext(ctx context.Context) (conn net.Conn, err error) {
	return c.dialer.DialContext(ctx, c.network, c.address)
}

var errDone = errors.New("done")

func (c *Client) do(parent context.Context, fn func(net.Conn) error) (err error) {
	var conn net.Conn
	if conn, err = c.dialContext(parent); err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(parent)

	eg.Go(func() (err error) {
		<-ctx.Done()

		if err = conn.Close(); err == nil {
			err = net.ErrClosed
		}

		return
	})

	eg.Go(func() (err error) {
		if err = fn(conn); err == nil {
			err = errDone
		}

		return
	})

	if err = eg.Wait(); errors.Is(err, errDone) {
		err = nil
	}

	return
}

func (c *Client) Kill(ctx context.Context) error {
	return c.do(ctx, func(conn net.Conn) error {
		return proto.Write(conn, "kill")
	})
}

type PingResponse struct {
	PID        int
	Version    semver.Version
	Background bool
}

func (c *Client) Ping(ctx context.Context) (res PingResponse, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "ping"); err != nil {
			return
		}

		var data []byte
		if data, err = proto.Read(conn); err != nil {
			return
		}

		if err = hasPrefix(data, "pong "); err == nil {
			err = json.Unmarshal(data[5:], &res)
		}

		return
	})

	return
}

func hasPrefix(data []byte, prefix string) (err error) {
	if !strings.HasPrefix(string(data), prefix) {
		format := fmt.Sprintf("invalid prefix: %%.%dq", len(prefix))

		err = fmt.Errorf(format, string(data))
	}

	return
}

type EstablishResponse struct {
	WireGuardState *wg.WireGuardState
	TunnelConfig   *wg.Config
}

func (c *Client) Establish(ctx context.Context, slug string) (res *EstablishResponse, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "establish", slug); err != nil {
			return
		}

		// this goes out to the API; don't time it out aggressively
		var data []byte
		if data, err = proto.Read(conn); err != nil {
			return
		}

		if err = hasPrefix(data, "ok "); err != nil {
			err = errors.New(string(data))

			return
		}

		res = &EstablishResponse{}
		if err = json.Unmarshal(data, res); err != nil {
			res = nil
		}

		return
	})

	return
}

func (c *Client) Probe(ctx context.Context, slug string) error {
	return c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "probe", slug); err != nil {
			return
		}

		var data []byte
		if data, err = proto.Read(conn); err != nil {
			return
		}

		if string(data) != "ok" {
			err = errors.New(string(data))
		}

		return
	})
}

func (c *Client) Resolve(ctx context.Context, slug, host string) (addr string, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "resolve", slug, host); err != nil {
			return
		}

		var data []byte
		if data, err = proto.Read(conn); err != nil {
			return
		}

		if err = hasPrefix(data, "ok "); err == nil {
			addr = string(data[3:])
		}

		return
	})

	return
}

func (c *Client) WaitForTunnel(ctx context.Context, org *api.Organization) (err error) {
	for {
		if err = c.Probe(ctx, org.Slug); !IsTunnelError(err) {
			break // we only reset on tunnel errors
		}

		pause.For(ctx, cycle)
	}

	return
}

func (c *Client) WaitForHost(ctx context.Context, org *api.Organization, host string) (err error) {
	for {
		if _, err = c.Resolve(ctx, org.Slug, host); !IsTunnelError(err) && !IsHostNotFoundError(err) {
			break
		}

		pause.For(ctx, cycle)
	}

	return
}

func (c *Client) Instances(ctx context.Context, org *api.Organization, app string) (instances Instances, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "instances", org.Slug, app); err != nil {
			return
		}

		// this goes out to the network; don't time it out aggressively
		var data []byte
		if data, err = proto.Read(conn); err != nil {
			return
		}

		if err = hasPrefix(data, "ok "); err == nil {
			err = json.Unmarshal(data[3:], &instances)
		}

		return
	})

	return
}

func (c *Client) Dialer(ctx context.Context, org *api.Organization) (d Dialer, err error) {
	var er *EstablishResponse
	if er, err = c.Establish(ctx, org.Slug); err != nil {
		d = &dialer{
			slug:   org.Slug,
			client: c,
			state:  er.WireGuardState,
			config: er.TunnelConfig,
		}
	}

	return
}

// TODO: refactor to struct
type Dialer interface {
	State() *wg.WireGuardState
	Config() *wg.Config
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type dialer struct {
	slug    string
	timeout time.Duration

	state  *wg.WireGuardState
	config *wg.Config

	client *Client
}

func (d *dialer) State() *wg.WireGuardState {
	return d.state
}

func (d *dialer) Config() *wg.Config {
	return d.config
}

func (d *dialer) DialContext(ctx context.Context, network, addr string) (conn net.Conn, err error) {
	if conn, err = d.client.dialContext(ctx); err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	timeout := strconv.FormatInt(int64(d.timeout), 10)
	if err = proto.Write(conn, "connect", d.slug, addr, timeout); err != nil {
		return
	}

	var data []byte
	if data, err = proto.Read(conn); err != nil {
		return
	}

	if string(data) != "ok" {
		err = mapError(errors.New(string(data)), d.slug, addr)

		return
	}

	return
}
