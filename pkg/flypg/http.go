package flypg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/superfly/flyctl/pkg/agent"
)

type Client struct {
	httpClient *http.Client
	BaseURL    string
}

// New creates an http client to the fly postgres http server running on port 5500
// over userland wireguard provided by the agent
func New(app string, dialer agent.Dialer) *Client {
	url := fmt.Sprintf("http://%s.internal:5500", app)

	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}

	client := &http.Client{Transport: tr}

	return &Client{
		httpClient: client,
		BaseURL:    url,
	}
}

// NewFromInstance creates a new Client that targets a specific instance(address)
func NewFromInstance(address string, dialer agent.Dialer) *Client {
	return New(address, dialer)
}

func (c *Client) Do(ctx context.Context, method, path string, in, out interface{}) error {
	req, err := c.NewRequest(path, method, in)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode > 299 {
		return newError(res.StatusCode, res)
	}

	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) NewRequest(path string, method string, in interface{}) (*http.Request, error) {
	var (
		body    io.Reader
		headers = make(map[string][]string)
	)

	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		headers = map[string][]string{
			"Content-Type": {"application/json"},
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	return req, nil
}
