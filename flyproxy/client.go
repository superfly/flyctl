package flyproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
)

type Client struct {
	baseUrl    *url.URL
	httpClient *http.Client
	userAgent  string
	logger     api.Logger
}

func New(ctx context.Context, orgSlug string) (*Client, error) {
	logger := logger.MaybeFromContext(ctx)

	client := client.FromContext(ctx).API()
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("fly-proxy: can't build tunnel for %s: %w", orgSlug, err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}

	httpClient, err := api.NewHTTPClient(logger, transport)
	if err != nil {
		return nil, fmt.Errorf("fly-proxy: can't setup HTTP client for %s: %w", orgSlug, err)
	}

	proxyBaseUrlString := fmt.Sprintf("http://[%s]:1729", resolvePeerIP(dialer.State().Peer.Peerip))
	proxyBaseUrl, err := url.Parse(proxyBaseUrlString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy url '%s' with error: %w", proxyBaseUrlString, err)
	}

	return &Client{
		baseUrl:    proxyBaseUrl,
		httpClient: httpClient,
		userAgent:  strings.TrimSpace(fmt.Sprintf("fly-cli/%s", buildinfo.Version())),
	}, nil
}

func (f *Client) Balance(ctx context.Context, appName string, opts *BalanceOptions) (*BalanceResponse, error) {
	out := new(BalanceResponse)

	endpoint := fmt.Sprintf("balance/%s", appName)
	query := []string{}

	if opts != nil {
		if opts.Destination != "" {
			query = append(query, fmt.Sprintf("dest=%s", opts.Destination))
		}
	}

	if len(query) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, strings.Join(query, "&"))
	}

	err := f.sendRequest(ctx, http.MethodGet, endpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance diagnostic for app %s: %w", appName, err)
	}
	return out, nil
}

func (f *Client) sendRequest(ctx context.Context, method, endpoint string, in, out interface{}, headers map[string][]string) error {
	req, err := f.NewRequest(ctx, method, endpoint, in, headers)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %v\n", err)
		}
	}()

	if resp.StatusCode > 299 {
		responseBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			responseBody = make([]byte, 0)
		}
		return &Error{
			OriginalError:      handleAPIError(resp.StatusCode, responseBody),
			ResponseStatusCode: resp.StatusCode,
			ResponseBody:       responseBody,
		}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}

func (f *Client) urlFromBaseUrl(pathAndQueryString string) (*url.URL, error) {
	newUrl := *f.baseUrl // this does a copy: https://github.com/golang/go/issues/38351#issue-597797864
	newPath, err := url.Parse(pathAndQueryString)
	if err != nil {
		return nil, fmt.Errorf("failed parsing proxy path '%s' with error: %w", pathAndQueryString, err)
	}
	return newUrl.ResolveReference(&url.URL{Path: newPath.Path, RawQuery: newPath.RawQuery}), nil
}

func (f *Client) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	var body io.Reader

	if headers == nil {
		headers = make(map[string][]string)
	}

	targetEndpoint, err := f.urlFromBaseUrl(fmt.Sprintf("/v1/%s", path))
	if err != nil {
		return nil, err
	}

	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		headers["Content-Type"] = []string{"application/json"}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetEndpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("could not create new request, %w", err)
	}
	req.Header = headers

	return req, nil
}

func handleAPIError(statusCode int, responseBody []byte) error {
	switch statusCode / 100 {
	case 1, 3:
		return fmt.Errorf("API returned unexpected status, %d", statusCode)
	case 4, 5:
		apiErr := struct {
			Error string `json:"error"`
		}{}
		if err := json.Unmarshal(responseBody, &apiErr); err != nil {
			return fmt.Errorf("request returned non-2xx status, %d", statusCode)
		}
		return errors.New(apiErr.Error)
	default:
		return errors.New("something went terribly wrong")
	}
}

func resolvePeerIP(ip string) string {
	peerIP := net.ParseIP(ip)
	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	return net.IP(natsIPBytes[:]).String()
}
