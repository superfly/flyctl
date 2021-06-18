// +build !linux

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

	"github.com/superfly/flyctl/api"
)

type Client struct {
	path string
}

const (
	defaultTimeout = 1500 * time.Millisecond
)

var (
	ErrUnreachable = errors.New("can't connect to agent")
)

func NewClient(path string) (*Client, error) {
	c := &Client{
		path: path,
	}

	testConn, err := c.connect()
	if err != nil {
		return nil, err
	}

	testConn.Close()

	return c, nil
}

func DefaultClient() (*Client, error) {
	return NewClient(fmt.Sprintf("%s/.fly/fly-agent.sock", os.Getenv("HOME")))
}

func (c *Client) connect() (net.Conn, error) {
	d := net.Dialer{
		Timeout: defaultTimeout,
	}

	conn, err := d.Dial("unix", c.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, err)
	}

	return conn, nil
}

func (c *Client) withConnection(f func(conn net.Conn) error) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	return f(conn)
}

func (c *Client) Kill() error {
	return c.withConnection(func(conn net.Conn) error {
		return writef(conn, "kill")
	})
}

func (c *Client) Ping() (int, error) {
	var pid int

	err := c.withConnection(func(conn net.Conn) error {
		writef(conn, "ping")

		conn.SetReadDeadline(time.Now().Add(defaultTimeout))

		pong, err := read(conn)
		if err != nil {
			return err
		}

		tup := strings.Split(string(pong), " ")
		if len(tup) != 2 {
			return fmt.Errorf("malformed response (no pid)")
		}

		pid, err = strconv.Atoi(tup[1])
		if err != nil {
			return fmt.Errorf("malformed response (bad pid: %w)", err)
		}

		return nil
	})

	return pid, err
}

func (c *Client) Establish(slug string) error {
	return c.withConnection(func(conn net.Conn) error {
		writef(conn, "establish %s", slug)

		// this goes out to the API; don't time it out aggressively
		reply, err := read(conn)
		if err != nil {
			return err
		}

		if string(reply) != "ok" {
			return fmt.Errorf("establish failed: %s", string(reply))
		}

		return nil
	})
}

func (c *Client) Probe(o *api.Organization) error {
	return c.withConnection(func(conn net.Conn) error {
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

func (c *Client) Instances(o *api.Organization, app string) (*Instances, error) {
	var instances *Instances

	err := c.withConnection(func(conn net.Conn) error {
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

type Dialer struct {
	Org     *api.Organization
	Timeout time.Duration

	client *Client
}

func (c *Client) Dialer(o *api.Organization) (*Dialer, error) {
	if err := c.Establish(o.Slug); err != nil {
		return nil, err
	}

	return &Dialer{
		Org:    o,
		client: c,
	}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.client.connect()
	if err != nil {
		return nil, err
	}

	writef(conn, "connect %s %s %d", d.Org.Slug, addr, d.Timeout)

	res, err := read(conn)
	if err != nil {
		return nil, err
	}

	if string(res) != "ok" {
		return nil, fmt.Errorf("got error reply from agent: %s", string(res))
	}

	return conn, nil
}
