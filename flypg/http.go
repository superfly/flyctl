package flypg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/terminal"
)

type Client struct {
	httpClient *http.Client
	BaseURL    string
}

// NewFromInstance creates a new Client that targets a specific instance(address)
func NewFromInstance(address string, dialer agent.Dialer) *Client {
	url := fmt.Sprintf("http://%s:5500", address)
	terminal.Debugf("flypg will connect to: %s\n", url)
	return &Client{
		httpClient: newHttpClient(dialer),
		BaseURL:    url,
	}
}

func newHttpClient(dialer agent.Dialer) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}

	retry := rehttp.NewTransport(
		transport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryAny(
				rehttp.RetryTemporaryErr(),
				rehttp.RetryStatuses(502, 503),
			),
		),
		rehttp.ExpJitterDelay(100*time.Millisecond, 1*time.Second),
	)

	logging := &fly.LoggingTransport{
		InnerTransport: retry,
		Logger:         terminal.DefaultLogger,
	}

	return &http.Client{Transport: logging}
}

func (c *Client) doRequest(ctx context.Context, method, path string, in interface{}) (io.ReadCloser, error) {
	req, err := c.NewRequest(path, method, in)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode > 299 {
		return nil, newError(res.StatusCode, res)
	}

	return res.Body, nil
}

func (c *Client) Do(ctx context.Context, method, path string, in, out interface{}) error {
	body, err := c.doRequest(ctx, method, path, in)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}

	return json.NewDecoder(body).Decode(out)
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
