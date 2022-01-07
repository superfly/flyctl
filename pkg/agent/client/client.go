// Package client implements a client for agent servers.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/blang/semver"
)

// New returns a Client which uses the server listening at the given unix socket
// path.
func New(ctx context.Context, path string) (c *Client, err error) {
	c = &Client{
		http: buildHTTPClient(path),
	}

	if err = c.Ping(ctx); err != nil {
		c = nil
	}

	return
}

func buildHTTPClient(path string) *http.Client {
	var d net.Dialer

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", path)
			},
		},
	}
}

// Client wraps the functionality of an agent client.
type Client struct {
	http *http.Client
}

type Status struct {
	PID        int            `json:"pid"`
	Version    semver.Version `json:"version"`
	Background bool           `json:"background"`
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Status(ctx)
	return err
}

func (c *Client) Status(ctx context.Context) (s Status, err error) {
	err = c.do(ctx, &s, http.MethodGet, "/status", nil)

	return
}

func (c *Client) do(ctx context.Context, dst interface{}, method, path string, body io.Reader) (err error) {
	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, method, path, body); err != nil {
		return
	}

	var res *http.Response
	if res, err = c.http.Do(req); err != nil {
		return
	}
	defer func() {
		if e := res.Body.Close(); err == nil {
			err = e
		}
	}()

	switch res.StatusCode {
	case http.StatusOK:
		if dst != nil {
			err = json.NewDecoder(res.Body).Decode(dst)
		}
	case http.StatusRequestTimeout:
		err = context.DeadlineExceeded
	default:
		err = errUnsupportedStatusCode(res.StatusCode)
	}

	return
}

type errUnsupportedStatusCode int

func (err errUnsupportedStatusCode) Error() string {
	return fmt.Sprintf("unsupported status code %d", err)
}
