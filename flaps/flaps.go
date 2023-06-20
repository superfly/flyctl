package flaps

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
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/go-querystring/query"
	"github.com/samber/lo"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/instrument"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/terminal"
)

var NonceHeader = "fly-machine-lease-nonce"

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
		authToken:  config.FromContext(ctx).AccessToken,
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
		authToken:  config.FromContext(ctx).AccessToken,
		httpClient: httpClient,
		userAgent:  strings.TrimSpace(fmt.Sprintf("fly-cli/%s", buildinfo.Version())),
	}, nil
}

func (f *Client) CreateApp(ctx context.Context, name string, org string) (err error) {
	in := map[string]interface{}{
		"app_name": name,
		"org_slug": org,
	}

	err = f.sendRequest(ctx, http.MethodPost, "/apps", in, nil, nil)
	return
}

func (f *Client) Launch(ctx context.Context, builder api.LaunchMachineInput) (out *api.Machine, err error) {
	metrics.Started(ctx, "machine_launch")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_launch/duration")
	defer func() {
		metrics.Status(ctx, "machine_launch", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	out = new(api.Machine)
	if err := f.sendRequest(ctx, http.MethodPost, "", builder, out, nil); err != nil {
		return nil, fmt.Errorf("failed to launch VM: %w", err)
	}

	return out, nil
}

func (f *Client) Update(ctx context.Context, builder api.LaunchMachineInput, nonce string) (out *api.Machine, err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	metrics.Started(ctx, "machine_update")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_update/duration")
	defer func() {
		metrics.Status(ctx, "machine_update", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	endpoint := fmt.Sprintf("/%s", builder.ID)
	out = new(api.Machine)
	if err := f.sendRequest(ctx, http.MethodPost, endpoint, builder, out, headers); err != nil {
		return nil, fmt.Errorf("failed to update VM %s: %w", builder.ID, err)
	}
	return out, nil
}

func (f *Client) Start(ctx context.Context, machineID string, nonce string) (out *api.MachineStartResponse, err error) {
	startEndpoint := fmt.Sprintf("/%s/start", machineID)

	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	out = new(api.MachineStartResponse)

	metrics.Started(ctx, "machine_start")
	defer func() {
		metrics.Status(ctx, "machine_start", err == nil)
	}()

	if err := f.sendRequest(ctx, http.MethodPost, startEndpoint, nil, out, headers); err != nil {
		return nil, fmt.Errorf("failed to start VM %s: %w", machineID, err)
	}
	return out, nil
}

type waitQuerystring struct {
	InstanceId     string `url:"instance_id,omitempty"`
	TimeoutSeconds int    `url:"timeout,omitempty"`
	State          string `url:"state,omitempty"`
}

const proxyTimeoutThreshold = 60 * time.Second

func (f *Client) Wait(ctx context.Context, machine *api.Machine, state string, timeout time.Duration) (err error) {
	waitEndpoint := fmt.Sprintf("/%s/wait", machine.ID)
	if state == "" {
		state = "started"
	}
	version := machine.InstanceID
	if machine.Version != "" {
		version = machine.Version
	}
	if timeout > proxyTimeoutThreshold {
		timeout = proxyTimeoutThreshold
	}
	if timeout < 1*time.Second {
		timeout = 1 * time.Second
	}
	timeoutSeconds := int(timeout.Seconds())
	waitQs := waitQuerystring{
		InstanceId:     version,
		TimeoutSeconds: timeoutSeconds,
		State:          state,
	}
	qsVals, err := query.Values(waitQs)
	if err != nil {
		return fmt.Errorf("error making query string for wait request: %w", err)
	}
	waitEndpoint += fmt.Sprintf("?%s", qsVals.Encode())
	if err := f.sendRequest(ctx, http.MethodGet, waitEndpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to wait for VM %s in %s state: %w", machine.ID, state, err)
	}
	return
}

func (f *Client) Stop(ctx context.Context, in api.StopMachineInput, nonce string) (err error) {
	stopEndpoint := fmt.Sprintf("/%s/stop", in.ID)

	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	if err := f.sendRequest(ctx, http.MethodPost, stopEndpoint, in, nil, headers); err != nil {
		return fmt.Errorf("failed to stop VM %s: %w", in.ID, err)
	}
	return
}

func (f *Client) Restart(ctx context.Context, in api.RestartMachineInput, nonce string) (err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	restartEndpoint := fmt.Sprintf("/%s/restart?force_stop=%t", in.ID, in.ForceStop)

	if in.Timeout != 0 {
		restartEndpoint += fmt.Sprintf("&timeout=%d", in.Timeout)
	}

	if in.Signal != "" {
		restartEndpoint += fmt.Sprintf("&signal=%s", in.Signal)
	}

	if err := f.sendRequest(ctx, http.MethodPost, restartEndpoint, nil, nil, headers); err != nil {
		return fmt.Errorf("failed to restart VM %s: %w", in.ID, err)
	}
	return
}

func (f *Client) Get(ctx context.Context, machineID string) (*api.Machine, error) {
	getEndpoint := ""

	if machineID != "" {
		getEndpoint = fmt.Sprintf("/%s", machineID)
	}

	out := new(api.Machine)

	err := f.sendRequest(ctx, http.MethodGet, getEndpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) GetMany(ctx context.Context, machineIDs []string) ([]*api.Machine, error) {
	machines := make([]*api.Machine, 0, len(machineIDs))
	for _, id := range machineIDs {
		m, err := f.Get(ctx, id)
		if err != nil {
			return machines, err
		}
		machines = append(machines, m)
	}
	return machines, nil
}

func (f *Client) List(ctx context.Context, state string) ([]*api.Machine, error) {
	getEndpoint := ""

	if state != "" {
		getEndpoint = fmt.Sprintf("?%s", state)
	}

	out := make([]*api.Machine, 0)

	err := f.sendRequest(ctx, http.MethodGet, getEndpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}
	return out, nil
}

// ListActive returns only non-destroyed that aren't in a reserved process group.
func (f *Client) ListActive(ctx context.Context) ([]*api.Machine, error) {
	getEndpoint := ""

	machines := make([]*api.Machine, 0)

	err := f.sendRequest(ctx, http.MethodGet, getEndpoint, nil, &machines, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list active VMs: %w", err)
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return !m.IsReleaseCommandMachine() && !m.IsFlyAppsConsole() && m.IsActive()
	})

	return machines, nil
}

// ListFlyAppsMachines returns apps that are part of the Fly apps platform.
// Destroyed machines and console machines are excluded.
// Unlike other List functions, this function retries multiple times.
func (f *Client) ListFlyAppsMachines(ctx context.Context) ([]*api.Machine, *api.Machine, error) {
	allMachines := make([]*api.Machine, 0)
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 500 * time.Millisecond
	b.MaxElapsedTime = 5 * time.Second
	err := backoff.Retry(func() error {
		err := f.sendRequest(ctx, http.MethodGet, "", nil, &allMachines, nil)
		if err != nil {
			if errors.Is(err, FlapsErrorNotFound) {
				return err
			} else {
				return backoff.Permanent(err)
			}
		}
		return nil
	}, backoff.WithContext(b, ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list VMs even after retries: %w", err)
	}
	var releaseCmdMachine *api.Machine
	machines := make([]*api.Machine, 0)
	for _, m := range allMachines {
		if m.IsFlyAppsPlatform() && m.IsActive() && !m.IsFlyAppsReleaseCommand() && !m.IsFlyAppsConsole() {
			machines = append(machines, m)
		} else if m.IsFlyAppsReleaseCommand() {
			releaseCmdMachine = m
		}
	}
	return machines, releaseCmdMachine, nil
}

func (f *Client) Destroy(ctx context.Context, input api.RemoveMachineInput, nonce string) (err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	destroyEndpoint := fmt.Sprintf("/%s?kill=%t", input.ID, input.Kill)

	if err := f.sendRequest(ctx, http.MethodDelete, destroyEndpoint, nil, nil, headers); err != nil {
		return fmt.Errorf("failed to destroy VM %s: %w", input.ID, err)
	}

	return
}

func (f *Client) Kill(ctx context.Context, machineID string) (err error) {
	in := map[string]interface{}{
		"signal": 9,
	}
	err = f.sendRequest(ctx, http.MethodPost, fmt.Sprintf("/%s/signal", machineID), in, nil, nil)

	if err != nil {
		return fmt.Errorf("failed to kill VM %s: %w", machineID, err)
	}
	return
}

func (f *Client) FindLease(ctx context.Context, machineID string) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	out := new(api.MachineLease)

	err := f.sendRequest(ctx, http.MethodGet, endpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) AcquireLease(ctx context.Context, machineID string, ttl *int) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	if ttl != nil {
		endpoint += fmt.Sprintf("?ttl=%d", *ttl)
	}

	out := new(api.MachineLease)

	err := f.sendRequest(ctx, http.MethodPost, endpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	terminal.Debugf("got lease on machine %s: %v\n", machineID, out)
	return out, nil
}

func (f *Client) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)
	if ttl != nil {
		endpoint += fmt.Sprintf("?ttl=%d", *ttl)
	}
	headers := make(map[string][]string)
	headers[NonceHeader] = []string{nonce}
	out := new(api.MachineLease)
	err := f.sendRequest(ctx, http.MethodPost, endpoint, nil, out, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	terminal.Debugf("got lease on machine %s: %v\n", machineID, out)
	return out, nil
}

func (f *Client) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	headers := make(map[string][]string)

	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	return f.sendRequest(ctx, http.MethodDelete, endpoint, nil, nil, headers)
}

func (f *Client) Exec(ctx context.Context, machineID string, in *api.MachineExecRequest) (*api.MachineExecResponse, error) {
	endpoint := fmt.Sprintf("/%s/exec", machineID)

	out := new(api.MachineExecResponse)

	err := f.sendRequest(ctx, http.MethodPost, endpoint, in, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to exec on VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) GetProcesses(ctx context.Context, machineID string) (api.MachinePsResponse, error) {
	endpoint := fmt.Sprintf("/%s/ps", machineID)

	var out api.MachinePsResponse

	err := f.sendRequest(ctx, http.MethodGet, endpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get processes from VM %s: %w", machineID, err)
	}

	return out, nil
}

func (f *Client) Cordon(ctx context.Context, machineID string) (err error) {
	metrics.Started(ctx, "machine_cordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_cordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_cordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	if err := f.sendRequest(ctx, http.MethodPost, fmt.Sprintf("/%s/cordon", machineID), nil, nil, nil); err != nil {
		return fmt.Errorf("failed to cordon VM: %w", err)
	}

	return nil
}

func (f *Client) UnCordon(ctx context.Context, machineID string) (err error) {
	metrics.Started(ctx, "machine_uncordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_uncordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_uncordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	if err := f.sendRequest(ctx, http.MethodPost, fmt.Sprintf("/%s/uncordon", machineID), nil, nil, nil); err != nil {
		return fmt.Errorf("failed to uncordon VM: %w", err)
	}

	return nil
}

func (f *Client) sendRequest(ctx context.Context, method, endpoint string, in, out interface{}, headers map[string][]string) error {
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

	if resp.StatusCode > 299 {
		responseBody, err := ioutil.ReadAll(resp.Body)
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

	targetEndpoint, err := f.urlFromBaseUrl(fmt.Sprintf("/v1/apps/%s/machines%s", f.appName, path))
	if err != nil {
		return nil, err
	}

	// fmt.Println("[targetEndpoint]]", targetEndpoint)

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
