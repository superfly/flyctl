package agent

import (
	"bytes"
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

	c := newClient("unix", PathToSocket())

	res, err := c.Ping(ctx)
	if err != nil {
		return StartDaemon(ctx)
	}

	if buildinfo.Version().EQ(res.Version) {
		return c, nil
	}

	// TOOD: log this instead
	msg := fmt.Sprintf("The running flyctl background agent (v%s) is older than the current flyctl (v%s).", buildinfo.Version(), res.Version)

	logger := logger.MaybeFromContext(ctx)
	if logger != nil {
		logger.Warn(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}

	if !res.Background {
		return c, nil
	}

	const stopMessage = "The out-of-date agent will be shut down along with existing wireguard connections. The new agent will start automatically as needed."
	if logger != nil {
		logger.Warn(stopMessage)
	} else {
		fmt.Fprintln(os.Stderr, stopMessage)
	}

	if err := c.Kill(ctx); err != nil {
		err = fmt.Errorf("failed stopping agent: %w", err)

		if logger != nil {
			logger.Error(err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}

		return nil, err
	}

	// this is gross, but we need to wait for the agent to exit
	pause.For(ctx, time.Second)

	return StartDaemon(ctx)
}

func newClient(network, addr string) *Client {
	return &Client{
		network: network,
		address: addr,
	}
}

func Dial(ctx context.Context, network, addr string) (client *Client, err error) {
	client = newClient(network, addr)

	if _, err = client.Ping(ctx); err != nil {
		client = nil
	}

	return
}

func DefaultClient(ctx context.Context) (*Client, error) {
	return Dial(ctx, "unix", PathToSocket())
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

type errInvalidResponse []byte

func (err errInvalidResponse) Error() string {
	return fmt.Sprintf("invalid server response: %q", string(err))
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

		if isOK(data) {
			err = unmarshal(&res, data)
		} else {
			err = errInvalidResponse(data)
		}

		return
	})

	return
}

const okPrefix = "ok "

func isOK(data []byte) bool {
	return isPrefixedWith(data, okPrefix)
}

func extractOK(data []byte) []byte {
	return data[len(okPrefix):]
}

const errorPrefix = "err "

func isError(data []byte) bool {
	return isPrefixedWith(data, errorPrefix)
}

func extractError(data []byte) error {
	msg := data[len(errorPrefix):]

	return errors.New(string(msg))
}

func isPrefixedWith(data []byte, prefix string) bool {
	return strings.HasPrefix(string(data), prefix)
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

		switch {
		default:
			err = errInvalidResponse(data)
		case isOK(data):
			res = &EstablishResponse{}
			if err = unmarshal(res, data); err != nil {
				res = nil
			}
		case isError(data):
			err = extractError(data)
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

		switch {
		default:
			err = errInvalidResponse(data)
		case string(data) == "ok":
			return // up and running
		case isError(data):
			err = extractError(data)
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

		switch {
		default:
			err = errInvalidResponse(data)
		case string(data) == "ok":
			err = ErrNoSuchHost
		case isOK(data):
			addr = string(extractOK(data))
		case isError(data):
			err = extractError(data)
		}

		return
	})

	return
}

func (c *Client) WaitForTunnel(ctx context.Context, slug string) (err error) {
	for {
		pause.For(ctx, cycle)

		if err = c.Probe(ctx, slug); !errors.Is(err, ErrTunnelUnavailable) {
			break
		}
	}

	return
}

func (c *Client) WaitForHost(ctx context.Context, slug, host string) (err error) {
	for {
		pause.For(ctx, cycle)

		if _, err = c.Resolve(ctx, slug, host); !errors.Is(err, ErrTunnelUnavailable) && !errors.Is(err, ErrNoSuchHost) {
			break
		}
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

		switch {
		default:
			err = errInvalidResponse(data)
		case isOK(data):
			err = unmarshal(&instances, data)
		case isError(data):
			err = extractError(data)
		}

		return
	})

	return
}

func unmarshal(dst interface{}, data []byte) (err error) {
	src := bytes.NewReader(extractOK(data))

	dec := json.NewDecoder(src)
	if err = dec.Decode(dst); err != nil {
		err = fmt.Errorf("failed decoding response: %w", err)
	}

	return
}

func (c *Client) Dialer(ctx context.Context, slug string) (d Dialer, err error) {
	var er *EstablishResponse
	if er, err = c.Establish(ctx, slug); err == nil {
		d = &dialer{
			slug:   slug,
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

	switch {
	default:
		err = errInvalidResponse(data)
	case string(data) == "ok":
		break
	case isError(data):
		err = extractError(data)
	}

	return
}
