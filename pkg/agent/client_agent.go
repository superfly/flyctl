// +build !windows

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/wg"
)

type agentClientProvider struct {
	path string
}

const (
	defaultTimeout = 1500 * time.Millisecond
)

var (
	ErrUnreachable = errors.New("can't connect to agent")
)

func newClientProvider(path string, api *api.Client) (clientProvider, error) {
	session := &agentClientProvider{path: path}

	testConn, err := session.connect()
	if err != nil {
		return nil, err
	}
	testConn.Close()

	return session, nil
}

func (c *agentClientProvider) connect() (net.Conn, error) {
	d := net.Dialer{
		Timeout: defaultTimeout,
	}

	conn, err := d.Dial("unix", c.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, err)
	}

	return conn, nil
}

func (c *agentClientProvider) withConnection(ctx context.Context, f func(conn net.Conn) error) error {
	errCh := make(chan error, 1)

	go func() {
		conn, err := c.connect()
		if err != nil {
			errCh <- err
		}
		defer conn.Close()

		errCh <- f(conn)
	}()

	select {
	case <-ctx.Done():
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *agentClientProvider) Kill(ctx context.Context) error {
	return c.withConnection(ctx, func(conn net.Conn) error {
		return writef(conn, "kill")
	})
}

func (c *agentClientProvider) Ping(ctx context.Context) (PingResponse, error) {
	resp := &PingResponse{}

	err := c.withConnection(ctx, func(conn net.Conn) error {
		writef(conn, "ping")

		conn.SetReadDeadline(time.Now().Add(defaultTimeout))

		pong, err := read(conn)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(string(pong), "pong ") {
			return fmt.Errorf("ping failed: %s", string(pong))
		}

		if err := json.Unmarshal(pong[5:], resp); err != nil {
			return fmt.Errorf("malformed response: %s", err)
		}

		return nil
	})

	return *resp, err
}

func (c *agentClientProvider) Establish(ctx context.Context, slug string) (*EstablishResponse, error) {
	resp := &EstablishResponse{}

	err := c.withConnection(ctx, func(conn net.Conn) error {
		writef(conn, "establish %s", slug)

		// this goes out to the API; don't time it out aggressively
		reply, err := read(conn)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(string(reply), "ok ") {
			return fmt.Errorf("establish failed: %s", string(reply))
		}

		if err := json.Unmarshal(reply[3:], resp); err != nil {
			return fmt.Errorf("malformed response: %s", err)
		}

		return nil
	})

	return resp, err
}

func (c *agentClientProvider) Probe(ctx context.Context, o *api.Organization) error {
	return c.withConnection(ctx, func(conn net.Conn) error {
		writef(conn, "probe %s", o.Slug)

		reply, err := read(conn)
		if err != nil {
			return err
		}

		if string(reply) != "ok" {
			return fmt.Errorf("probe failed: %s", string(reply))
		}

		return nil
	})
}

func (c *agentClientProvider) Resolve(ctx context.Context, o *api.Organization, addr string) (resp string, err error) {
	err = c.withConnection(ctx, func(conn net.Conn) error {
		writef(conn, "resolve %s %s", o.Slug, addr)

		reply, err := read(conn)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(string(reply), "ok ") {
			return fmt.Errorf("resolve failed: %s", reply)
		}

		resp = string(reply[3:])
		return nil
	})

	return
}

func (c *agentClientProvider) Instances(ctx context.Context, o *api.Organization, app string) (*Instances, error) {
	var instances *Instances

	err := c.withConnection(ctx, func(conn net.Conn) error {
		writef(conn, "instances %s %s", o.Slug, app)

		// this goes out to the network; don't time it out aggressively
		reply, err := read(conn)
		if err != nil {
			return err
		}

		if string(reply[0:3]) != "ok " {
			return fmt.Errorf("failed to retrieve instances: %s", string(reply))
		}

		reply = reply[3:]

		inst := &Instances{}

		if err = json.NewDecoder(bytes.NewReader(reply)).Decode(inst); err != nil {
			return fmt.Errorf("failed to retrieve instances: malformed response: %s", err)
		}

		instances = inst

		return nil
	})

	return instances, err
}

func (c *agentClientProvider) Dialer(ctx context.Context, o *api.Organization) (Dialer, error) {
	resp, err := c.Establish(ctx, o.Slug)
	if err != nil {
		return nil, err
	}

	return &agentDialer{
		Org:     o,
		session: c,
		state:   resp.WireGuardState,
		config:  resp.TunnelConfig,
	}, nil
}

type agentDialer struct {
	Org     *api.Organization
	Timeout time.Duration

	state  *wg.WireGuardState
	config *wg.Config

	session *agentClientProvider
}

func (d *agentDialer) State() *wg.WireGuardState {
	return d.state
}

func (d *agentDialer) Config() *wg.Config {
	return d.config
}

func (d *agentDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.session.connect()
	if err != nil {
		return nil, err
	}

	writef(conn, "connect %s %s %d", d.Org.Slug, addr, d.Timeout)

	res, err := read(conn)
	if err != nil {
		return nil, err
	}

	if string(res) != "ok" {
		return nil, mapResolveError(errors.New(string(res)), d.Org.Slug, addr)
	}

	return conn, nil
}
