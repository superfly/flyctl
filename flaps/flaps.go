package flaps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/superfly/flyctl/internal/command_context"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/sentry"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/instrument"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
)

const headerFlyRequestId = "fly-request-id"

type Client struct {
	appName    string
	baseUrl    *url.URL
	authToken  string
	httpClient *http.Client
	userAgent  string
}

func New(ctx context.Context, app *api.AppCompact) (*Client, error) {
	return NewWithOptions(ctx, NewClientOpts{AppCompact: app, AppName: app.Name})
}

func NewFromAppName(ctx context.Context, appName string) (*Client, error) {
	return NewWithOptions(ctx, NewClientOpts{AppName: appName})
}

type NewClientOpts struct {
	// required:
	AppName string

	// optional, avoids API roundtrip when connecting to flaps by wireguard:
	AppCompact *api.AppCompact

	// optional:
	Logger api.Logger
}

func NewWithOptions(ctx context.Context, opts NewClientOpts) (*Client, error) {
	// FIXME: do this once we setup config for `fly config ...` commands, and then use cfg.FlapsBaseURL below
	// cfg := config.FromContext(ctx)
	var err error
	flapsBaseURL := os.Getenv("FLY_FLAPS_BASE_URL")
	if strings.TrimSpace(strings.ToLower(flapsBaseURL)) == "peer" {
		orgSlug, err := resolveOrgSlugForApp(ctx, opts.AppCompact, opts.AppName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve org for app '%s': %w", opts.AppName, err)
		}
		return newWithUsermodeWireguard(ctx, wireguardConnectionParams{
			appName: opts.AppName,
			orgSlug: orgSlug,
		})
	} else if flapsBaseURL == "" {
		flapsBaseURL = "https://api.machines.dev"
	}
	flapsUrl, err := url.Parse(flapsBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid FLY_FLAPS_BASE_URL '%s' with error: %w", flapsBaseURL, err)
	}
	var logger api.Logger = logger.MaybeFromContext(ctx)
	if opts.Logger != nil {
		logger = opts.Logger
	}
	httpClient, err := api.NewHTTPClient(logger, httptracing.NewTransport(http.DefaultTransport))
	if err != nil {
		return nil, fmt.Errorf("flaps: can't setup HTTP client to %s: %w", flapsUrl.String(), err)
	}
	return &Client{
		appName:    opts.AppName,
		baseUrl:    flapsUrl,
		authToken:  config.Tokens(ctx).Flaps(),
		httpClient: httpClient,
		userAgent:  strings.TrimSpace(fmt.Sprintf("fly-cli/%s", buildinfo.Version())),
	}, nil
}

func resolveOrgSlugForApp(ctx context.Context, app *api.AppCompact, appName string) (string, error) {
	app, err := resolveApp(ctx, app, appName)
	if err != nil {
		return "", err
	}
	return app.Organization.Slug, nil
}

func resolveApp(ctx context.Context, app *api.AppCompact, appName string) (*api.AppCompact, error) {
	var err error
	if app == nil {
		client := client.FromContext(ctx).API()
		app, err = client.GetAppCompact(ctx, appName)
	}
	return app, err
}

type wireguardConnectionParams struct {
	appName string
	orgSlug string
}

func newWithUsermodeWireguard(ctx context.Context, params wireguardConnectionParams) (*Client, error) {
	logger := logger.MaybeFromContext(ctx)

	client := client.FromContext(ctx).API()
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, params.orgSlug)
	if err != nil {
		return nil, fmt.Errorf("flaps: can't build tunnel for %s: %w", params.orgSlug, err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}

	httpClient, err := api.NewHTTPClient(logger, httptracing.NewTransport(transport))
	if err != nil {
		return nil, fmt.Errorf("flaps: can't setup HTTP client for %s: %w", params.orgSlug, err)
	}

	flapsBaseUrlString := fmt.Sprintf("http://[%s]:4280", resolvePeerIP(dialer.State().Peer.Peerip))
	flapsBaseUrl, err := url.Parse(flapsBaseUrlString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse flaps url '%s' with error: %w", flapsBaseUrlString, err)
	}

	return &Client{
		appName:    params.appName,
		baseUrl:    flapsBaseUrl,
		authToken:  config.Tokens(ctx).Flaps(),
		httpClient: httpClient,
		userAgent:  strings.TrimSpace(fmt.Sprintf("fly-cli/%s", buildinfo.Version())),
	}, nil
}

func (f *Client) CreateApp(ctx context.Context, name string, org string) (err error) {
	in := map[string]interface{}{
		"app_name": name,
		"org_slug": org,
	}

	err = f._sendRequest(ctx, http.MethodPost, "/apps", in, nil, nil)
	return
}

func (f *Client) _sendRequest(ctx context.Context, method, endpoint string, in, out interface{}, headers map[string][]string) error {
	timing := instrument.Flaps.Begin()
	defer timing.End()

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

	// This tries to find the specific flaps call (using a regex) being made, and then sends it up as a metric
	defer sendFlapsCallMetric(ctx, endpoint, timing, resp.StatusCode)

	if resp.StatusCode > 299 {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			responseBody = make([]byte, 0)
		}
		return &FlapsError{
			OriginalError:      handleAPIError(resp.StatusCode, responseBody),
			ResponseStatusCode: resp.StatusCode,
			ResponseBody:       responseBody,
			FlyRequestId:       resp.Header.Get(headerFlyRequestId),
		}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}

var flapsCallRegex = regexp.MustCompile(`\/(?P<machineId>[a-z-A-Z0-9]*)\/(?P<flapsCall>[a-zA-Z0-9]*)`)

type flapsCall struct {
	Call       string  `json:"c"`
	Command    string  `json:"co"`
	Duration   float64 `json:"d"`
	StatusCode int     `json:"s"`
}

func sendFlapsCallMetric(ctx context.Context, endpoint string, timing instrument.CallTimer, statusCode int) {

	matches := flapsCallRegex.FindStringSubmatch(endpoint)
	index := flapsCallRegex.SubexpIndex("flapsCall")
	machineIndex := flapsCallRegex.SubexpIndex("machineId")
	machineCallOnly := false

	// unless something changes about flaps, this should always be false (meaning the regex passes)
	if index >= len(matches) {
		if machineIndex < len(matches) {
			machineCallOnly = true
		} else {
			err := fmt.Errorf("flaps endpoint %s did not match regex %s", endpoint, flapsCallRegex.String())
			sentry.CaptureException(err)
			return
		}
	}

	call := matches[index]

	if machineCallOnly {
		call = "machine_call"
	}

	cmdCtx := command_context.FromContext(ctx)
	var nameParts []string
	for cmdCtx != nil {
		if cmdCtx.Name() == "flyctl" {
			break
		}
		nameParts = append([]string{cmdCtx.Name()}, nameParts...)
		cmdCtx = cmdCtx.Parent()
	}
	commandName := strings.Join(nameParts, "-")

	metrics.Send(ctx, "flaps_call", flapsCall{
		Call:       call,
		Command:    commandName,
		Duration:   time.Since(timing.Start).Seconds(),
		StatusCode: statusCode,
	})
}

func (f *Client) urlFromBaseUrl(pathAndQueryString string) (*url.URL, error) {
	newUrl := *f.baseUrl // this does a copy: https://github.com/golang/go/issues/38351#issue-597797864
	newPath, err := url.Parse(pathAndQueryString)
	if err != nil {
		return nil, fmt.Errorf("failed parsing flaps path '%s' with error: %w", pathAndQueryString, err)
	}
	return newUrl.ResolveReference(&url.URL{Path: newPath.Path, RawQuery: newPath.RawQuery}), nil
}

func (f *Client) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	var body io.Reader

	if headers == nil {
		headers = make(map[string][]string)
	}

	targetEndpoint, err := f.urlFromBaseUrl(fmt.Sprintf("/v1%s", path))
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

	req.Header.Add("Authorization", api.AuthorizationHeader(f.authToken))

	return req, nil
}

func handleAPIError(statusCode int, responseBody []byte) error {
	switch statusCode / 100 {
	case 1, 3:
		return fmt.Errorf("API returned unexpected status, %d", statusCode)
	case 4, 5:
		apiErr := struct {
			Error   string `json:"error"`
			Message string `json:"message,omitempty"`
		}{}
		if err := json.Unmarshal(responseBody, &apiErr); err != nil {
			return fmt.Errorf("request returned non-2xx status, %d", statusCode)
		}
		if apiErr.Message != "" {
			return fmt.Errorf("%s", apiErr.Message)
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
