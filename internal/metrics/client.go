package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewClient(ctx context.Context) (*Client, error) {
	token, err := getMetricsToken(ctx)
	if err != nil {
		return nil, err
	}

	cfg := config.FromContext(ctx)
	logger := logger.FromContext(ctx)

	return &Client{
		token:      token,
		baseURL:    cfg.MetricsBaseURL,
		httpClient: newHttpClient(logger),
	}, nil
}

func (c *Client) Send(ctx context.Context, entry []Entry) error {
	var path = "/metrics_post"

	return c.do(ctx, "POST", path, entry, nil, nil)
}

func (c *Client) do(ctx context.Context, method, endpoint string, in, out interface{}, headers map[string][]string) error {
	req, err := c.newRequest(ctx, method, endpoint, in, headers)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode > 299 {
		var buf bytes.Buffer
		_, err := io.Copy(&buf, resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		return handleAPIError(resp.StatusCode, buf.Bytes())
	}

	if out != nil {
		err = json.NewDecoder(resp.Body).Decode(out)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	var (
		body io.Reader
		url  = c.baseURL + path
	)

	if headers == nil {
		headers = make(map[string][]string)
	}

	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		headers["Content-Type"] = []string{"application/json"}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)

	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header[k] = v
	}

	req.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Version().String()))
	req.Header.Set("Authorization", c.token)

	return req, nil
}

func newHttpClient(logger *logger.Logger) *http.Client {
	retry := rehttp.NewTransport(
		http.DefaultTransport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryAny(
				rehttp.RetryTemporaryErr(),
				rehttp.RetryStatuses(502, 503, 500),
			),
		),
		rehttp.ExpJitterDelay(100*time.Millisecond, 1*time.Second),
	)

	logging := &api.LoggingTransport{
		InnerTransport: retry,
		Logger:         logger,
	}

	return &http.Client{Transport: logging}
}

func handleAPIError(statusCode int, responseBody []byte) error {
	switch statusCode / 100 {
	case 1, 3, 4, 5:
		return fmt.Errorf("API returned unexpected status, %d", statusCode)
	default:
		return errors.New("something went terribly wrong")
	}
}
