package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/azazeal/pause"
	"github.com/blang/semver"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/agent/internal/proto"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/wg"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/flag"
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
	msg := fmt.Sprintf("The running flyctl agent (v%s) is older than the current flyctl (v%s).", res.Version, buildinfo.Version())

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

var ErrAgentNotRunning = errors.New("agent not running")

func Dial(ctx context.Context, network, addr string) (*Client, error) {
	client := newClient(network, addr)

	if _, err := client.Ping(ctx); err != nil {
		// if the agen't isn't running the error will be "connect: file or directory not found"
		// catch it and return a sentinel error
		var syscallErr *os.SyscallError
		if errors.As(err, &syscallErr) && syscallErr.Err == syscall.ENOENT {
			return nil, ErrAgentNotRunning
		}
		return nil, err
	}

	return client, nil
}

func DefaultClient(ctx context.Context) (*Client, error) {
	return Dial(ctx, "unix", PathToSocket())
}

const (
	cycle = time.Second / 20
)

type Client struct {
	network string
	address string
	dialer  net.Dialer
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

func (c *Client) doEstablish(ctx context.Context, slug string, recycle bool) (res *EstablishResponse, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		verb := "establish"
		if recycle {
			verb = "reestablish"
		}

		if err = proto.Write(conn, verb, slug); err != nil {
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

func (c *Client) Establish(ctx context.Context, slug string) (res *EstablishResponse, err error) {
	return c.doEstablish(ctx, slug, false)
}

func (c *Client) Reestablish(ctx context.Context, slug string) (res *EstablishResponse, err error) {
	return c.doEstablish(ctx, slug, true)
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

// WaitForTunnel waits for a tunnel to the given org slug to become available
// in the next four minutes.
func (c *Client) WaitForTunnel(parent context.Context, slug string) (err error) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()

	for {
		if err = c.Probe(ctx, slug); !errors.Is(err, ErrTunnelUnavailable) {
			break
		}

		pause.For(ctx, cycle)
	}

	if parent.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		err = ErrTunnelUnavailable
	}

	return
}

// WaitForDNS waits for a Fly host internal DNS entry to register
func (c *Client) WaitForDNS(parent context.Context, dialer Dialer, slug string, host string) (err error) {
	io := iostreams.FromContext(parent)

	if !flag.GetBool(parent, "quiet") {
		io.StartProgressIndicatorMsg(fmt.Sprintf("Waiting for host %s", host))
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	if !flag.GetBool(parent, "quiet") {
		io.StopProgressIndicator()
	}

	for {
		if _, err = c.Resolve(ctx, slug, host); !errors.Is(err, ErrNoSuchHost) {
			break
		}

		pause.For(ctx, cycle)
	}

	if parent.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		err = ErrNoSuchHost
	}

	return
}

func (c *Client) Instances(ctx context.Context, org, app string) (instances Instances, err error) {
	err = c.do(ctx, func(conn net.Conn) (err error) {
		if err = proto.Write(conn, "instances", org, app); err != nil {
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

// Dialer establishes a connection to the wireguard agent and return a dialier
// for use in subsequent actions, such as running ssh commands or opening proxies
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

// ConnectToTunnel is a convenience method for connect to a wireguard tunnel
// and returning a Dialer. Only suitable for use in the new CLI commands.
func (c *Client) ConnectToTunnel(ctx context.Context, slug string) (d Dialer, err error) {
	io := iostreams.FromContext(ctx)

	dialer, err := c.Dialer(ctx, slug)
	if err != nil {
		return nil, err
	}
	io.StartProgressIndicatorMsg(fmt.Sprintf("Opening a wireguard tunnel to %s", slug))
	if err := c.WaitForTunnel(ctx, slug); err != nil {
		return nil, fmt.Errorf("tunnel unavailable for organization %s: %w", slug, err)
	}
	io.StopProgressIndicator()
	return dialer, err
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

// Pinger wraps a connection to the flyctl agent over which ICMP
// requests and replies are written. There's a simple protocol
// for encapsulating requests and responses; drive it with the Pinger
// member functions. Pinger implements most of net.PacketConn but is
// not really intended as such.
type Pinger struct {
	c   net.Conn
	err error
}

// Pinger creates a Pinger struct. It does this by first ensuring
// a WireGuard session exists for the specified org, and then
// opening an additional connection to the agent, which is upgraded
// to a Pinger connection by sending the "ping6" command. Call "Close"
// on a Pinger when you're done pinging things.
func (c *Client) Pinger(ctx context.Context, slug string) (p *Pinger, err error) {
	if _, err = c.Establish(ctx, slug); err != nil {
		return nil, fmt.Errorf("pinger: %w", err)
	}

	conn, err := c.dialContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("pinger: %w", err)
	}

	if err = proto.Write(conn, "ping6", slug); err != nil {
		return nil, fmt.Errorf("pinger: %w", err)
	}

	return &Pinger{c: conn}, nil
}

func (p *Pinger) SetReadDeadline(t time.Time) error {
	return p.c.SetReadDeadline(t)
}

func (p *Pinger) Close() error {
	return p.c.Close()
}

// Err returns any non-recoverable error seen on this Pinger connection;
// WriteTo and ReadFrom on a Pinger will not function if Err returns
// non-nil.
func (p *Pinger) Err() error {
	return p.err
}

// WriteTo writes an ICMP message, including headers, to the specified
// address. `addr` should always be an IPv6 net.IPAddr beginning with
// `fdaa` --- you cannot ping random hosts on the Internet with this
// interface. See golang/x/net/icmp for message construction details;
// this interface uses gVisor netstack, which is fussy about ICMP,
// and will only allow icmp.Echo messages with a code of 0.
//
// Pinger runs a trivial protocol to encapsulate ICMP messages over
// agent connections: each message is a 16-byte IPv6 address, followed
// by an NBO u16 length, followed by the ICMP message bytes, which
// again must begin with an ICMP header. Checksums are performed by
// netstack; don't bother with them.
func (p *Pinger) WriteTo(buf []byte, addr net.Addr) (int64, error) {
	if p.err != nil {
		return 0, p.err
	}

	if len(buf) >= 1500 {
		return 0, fmt.Errorf("icmp write: too large (>=1500 bytes)")
	}

	var v6addr net.IP

	ipaddr, ok := addr.(*net.IPAddr)
	if ok {
		v6addr = ipaddr.IP.To16()
	}

	if !ok || v6addr == nil {
		return 0, fmt.Errorf("icmp write: bad address type")
	}

	_, err := p.c.Write([]byte(v6addr))
	if err != nil {
		p.err = fmt.Errorf("icmp write: address: %w", err)
		return 0, p.err
	}

	lbuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lbuf, uint16(len(buf)))

	_, err = p.c.Write(lbuf)
	if err != nil {
		p.err = fmt.Errorf("icmp write: length: %w", err)
		return 0, p.err
	}

	_, err = p.c.Write(buf)
	if err != nil {
		p.err = fmt.Errorf("icmp write: payload: %w", err)
		return 0, p.err
	}

	return int64(len(buf)), nil
}

// ReadFrom reads an ICMP message from a Pinger, using the same
// protocol as WriteTo. Call `SetReadDeadline` to poll this
// interface while watching channels or whatever.
func (p *Pinger) ReadFrom(buf []byte) (int64, net.Addr, error) {
	if p.err != nil {
		return 0, nil, p.err
	}

	lbuf := make([]byte, 2)
	v6buf := make([]byte, 16)

	_, err := io.ReadFull(p.c, v6buf)
	if err != nil {
		// common case: read deadline set, this is just
		// a timeout, we don't want to close the pinger

		if !errors.Is(err, os.ErrDeadlineExceeded) {
			p.err = fmt.Errorf("icmp read: addr: %w", err)
			return 0, nil, p.err
		}

		return 0, nil, err
	}

	_, err = io.ReadFull(p.c, lbuf)
	if err != nil {
		p.err = fmt.Errorf("icmp read: length: %w", err)
		return 0, nil, p.err
	}

	paylen := binary.BigEndian.Uint16(lbuf)
	inbuf := make([]byte, paylen)

	_, err = io.ReadFull(p.c, inbuf)
	if err != nil {
		p.err = fmt.Errorf("icmp read: payload: %w", err)
		return 0, nil, p.err
	}

	// burning a copy just so i don't have to think about what
	// happens if you try to read 1 byte of a 1000-byte ping

	copy(buf, inbuf)

	return int64(paylen), &net.IPAddr{IP: net.IP(v6buf)}, nil
}
