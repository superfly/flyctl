package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
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

func (c *Client) Kill() error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	writef(conn, "kill")

	return nil
}

func (c *Client) Ping() (int, error) {
	conn, err := c.connect()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	writef(conn, "ping")

	conn.SetReadDeadline(time.Now().Add(defaultTimeout))

	pong, err := read(conn)
	if err != nil {
		return 0, err
	}

	tup := strings.Split(string(pong), " ")
	if len(tup) != 2 {
		return 0, fmt.Errorf("malformed response (no pid)")
	}

	pid, err := strconv.Atoi(tup[1])
	if err != nil {
		return 0, fmt.Errorf("malformed response (bad pid: %w)", err)
	}

	return pid, nil
}

func (c *Client) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	writef(conn, "connect %s %d", addr, 2000 /* whatever, for now */)

	conn.SetReadDeadline(time.Now().Add(defaultTimeout))

	res, err := read(conn)
	if err != nil {
		return nil, err
	}

	if string(res) != "ok" {
		return nil, fmt.Errorf("got error reply from agent: %s", string(res))
	}

	var t0 time.Time

	conn.SetReadDeadline(t0)

	return conn, nil
}
