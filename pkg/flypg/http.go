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

type Error struct {
	StatusCode int
	Err        string `json:"error"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.StatusCode, e.Err)
}

func newError(status int, res *http.Response) error {
	var e = new(Error)

	e.StatusCode = status

	switch res.Header.Get("Content-Type") {
	case "application/json":

		if err := json.NewDecoder(res.Body).Decode(e); err != nil {
			return err
		}
	default:
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		e.Err = string(b)
	}

	return e
}
