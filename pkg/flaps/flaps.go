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
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"

	"github.com/superfly/flyctl/flyctl"

	"github.com/superfly/flyctl/internal/client"
)

type Client struct {
	app        *api.AppCompact
	peerIP     string
	authToken  string
	httpClient *http.Client
}

func New(ctx context.Context, app *api.AppCompact) (*Client, error) {
	client := client.FromContext(ctx).API()
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	return &Client{
		app:        app,
		peerIP:     resolvePeerIP(dialer.State().Peer.Peerip),
		authToken:  flyctl.GetAPIToken(),
		httpClient: newHttpCLient(dialer),
	}, nil
}

func newHttpCLient(dialer agent.Dialer) *http.Client {
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
	logging := &LoggingTransport{
		innerTransport: retry,
		logger:         terminal.DefaultLogger,
	}

	return &http.Client{Transport: logging}
}

func (f *Client) Launch(ctx context.Context, builder api.LaunchMachineInput) (*api.V1Machine, error) {
	fmt.Println("Machine is launching...")

	var endpoint string
	if builder.ID != "" {
		endpoint = fmt.Sprintf("/%s", builder.ID)
	}

	var out = new(api.V1Machine)

	if err := f.sendRequest(ctx, http.MethodPost, endpoint, builder, out); err != nil {
		return nil, err
	}

	return out, nil
}

func (f *Client) Update(ctx context.Context, builder api.LaunchMachineInput) (*api.V1Machine, error) {
	fmt.Println("Machine is updating...")

	endpoint := fmt.Sprintf("/%s", builder.ID)

	var out = new(api.V1Machine)

	if err := f.sendRequest(ctx, http.MethodPost, endpoint, builder, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Client) Start(ctx context.Context, machineID string) (*api.MachineStartResponse, error) {
	fmt.Println("Machine is starting...")
	startEndpoint := fmt.Sprintf("/%s/start", machineID)

	out := new(api.MachineStartResponse)

	if err := f.sendRequest(ctx, http.MethodPost, startEndpoint, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Client) Wait(ctx context.Context, machine *api.V1Machine) error {
	fmt.Println("Waiting on firecracker VM...")

	waitEndpoint := fmt.Sprintf("/%s/wait", machine.ID)

	if machine.InstanceID != "" {
		waitEndpoint += fmt.Sprintf("?instance_id=%s", machine.InstanceID)
	}

	return f.sendRequest(ctx, http.MethodGet, waitEndpoint, nil, nil)
}

func (f *Client) Stop(ctx context.Context, machine api.V1MachineStop) error {
	stopEndpoint := fmt.Sprintf("/%s/stop", machine.ID)

	return f.sendRequest(ctx, http.MethodPost, stopEndpoint, nil, nil)
}

func (f *Client) Get(ctx context.Context, machineID string) (*api.V1Machine, error) {
	var getEndpoint = ""

	if machineID != "" {
		getEndpoint = fmt.Sprintf("/%s", machineID)
	}

	out := new(api.V1Machine)

	err := f.sendRequest(ctx, http.MethodGet, getEndpoint, nil, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Client) List(ctx context.Context, machineID string) ([]api.V1Machine, error) {
	var getEndpoint = ""

	if machineID != "" {
		getEndpoint = fmt.Sprintf("/%s", machineID)
	}

	out := make([]api.V1Machine, 0)

	err := f.sendRequest(ctx, http.MethodGet, getEndpoint, nil, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Client) Destroy(ctx context.Context, input api.RemoveMachineInput) error {
	destroyEndpoint := fmt.Sprintf("/%s?kill=%t", input.ID, input.Kill)

	return f.sendRequest(ctx, http.MethodDelete, destroyEndpoint, nil, nil)
}

func (f *Client) Kill(ctx context.Context, machineID string) error {

	var in = map[string]interface{}{
		"signal": 9,
	}
	err := f.sendRequest(ctx, http.MethodPost, fmt.Sprintf("/%s/signal", machineID), in, nil)
	if err != nil {
		return err
	}
	return nil
}

func (f *Client) Lease(ctx context.Context, machineID string) (*api.MachineLease, error) {
	var endpoint = fmt.Sprintf("/%s/lease", machineID)

	out := new(api.MachineLease)

	err := f.sendRequest(ctx, http.MethodPost, endpoint, nil, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Client) sendRequest(ctx context.Context, method, endpoint string, in, out interface{}) error {

	req, err := f.NewRequest(ctx, method, endpoint, in)
	if err != nil {
		return err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return handleAPIError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}

func (f *Client) NewRequest(ctx context.Context, method, path string, in interface{}) (*http.Request, error) {
	var (
		body    io.Reader
		peerIP  = f.peerIP
		headers = make(map[string][]string)
	)
	targetEndpoint := fmt.Sprintf("http://[%s]:4280/v1/apps/%s/machines%s", peerIP, f.app.Name, path)

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

	req, err := http.NewRequestWithContext(ctx, method, targetEndpoint, body)
	if err != nil {
		return nil, fmt.Errorf("could not create new request, %w", err)
	}
	req.Header = headers

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.authToken))

	return req, nil
}

func handleAPIError(resp *http.Response) error {
	switch resp.StatusCode / 100 {
	case 1, 3:
		return fmt.Errorf("API returned unexpected status, %d", resp.StatusCode)
	case 4, 5:
		apiErr := struct {
			Error   string `json:"error"`
			Message string `json:"message,omitempty"`
		}{}
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("request returned non-2xx status, %d", resp.StatusCode)
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
