// Package client implements a client for agent servers.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"

	"github.com/blang/semver"
)

// New returns a Client to the server listening at the given unix socket path.
func New(ctx context.Context, path string) (c *Client, err error) {
	c = &Client{
		http: buildHTTPClient(path),
	}

	if _, err = c.Status(ctx); err != nil {
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
	root *url.URL
}

type Status struct {
	PID        int            `json:"pid"`
	Version    semver.Version `json:"version"`
	Background bool           `json:"background"`
}

func (c *Client) Status(ctx context.Context) (s Status, err error) {
	err = c.request(ctx, &s, http.MethodGet, nil, "status")

	return
}

func (c *Client) request(ctx context.Context, into interface{}, method string, body io.Reader, segments ...string) (err error) {
	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, method, parseURL(segments...), body); err != nil {
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
	case http.StatusNoContent:
		break
	case http.StatusOK:
		err = json.NewDecoder(res.Body).Decode(into)
	case http.StatusRequestTimeout:
		err = context.DeadlineExceeded
	default:
		err = errUnsupportedStatusCode(res.StatusCode)
	}

	return
}

var root *url.URL

func init() {
	var err error
	if root, err = url.Parse("http://fly-agent/"); err != nil {
		panic(err)
	}
}

func parseURL(segments ...string) string {
	if len(segments) == 0 {
		return root.String()
	}

	url, err := root.Parse(path.Join(segments...))
	if err != nil {
		panic(err)
	}

	return url.String()
}

type errUnsupportedStatusCode int

func (err errUnsupportedStatusCode) Error() string {
	return fmt.Sprintf("unsupported status code %d", err)
}
