package flaps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/flyctl"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
)

type Client struct {
	app       *api.App
	peerIP    string
	authToken string
}

func New(ctx context.Context, app *api.App) (*Client, error) {
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
		app:       app,
		peerIP:    resolvePeerIP(dialer.State().Peer.Peerip),
		authToken: flyctl.GetAPIToken(),
	}, nil
}

func (f *Client) Launch(ctx context.Context, builder api.LaunchMachineInput) ([]byte, error) {
	fmt.Println("Machine is launching...")

	body, err := json.Marshal(builder)
	if err != nil {
		return nil, fmt.Errorf("machine failed to launch, %w", err)
	}

	return f.sendRequest(ctx, nil, http.MethodPost, "", body)
}

func (f *Client) Start(ctx context.Context, machineID string) ([]byte, error) {
	fmt.Println("Machine is starting...")
	startEndpoint := fmt.Sprintf("/%s/start", machineID)
	return f.sendRequest(ctx, nil, http.MethodPost, startEndpoint, nil)
}

func (f *Client) Wait(ctx context.Context, machine *api.V1Machine) ([]byte, error) {
	fmt.Println("Waiting on firecracker VM...")

	waitEndpoint := fmt.Sprintf("/%s/wait", machine.ID)

	if machine.InstanceID != "" {
		waitEndpoint = fmt.Sprintf("?instance_id=%s", machine.InstanceID)
	}

	return f.sendRequest(ctx, nil, http.MethodGet, waitEndpoint, nil)
}

func (f *Client) Stop(ctx context.Context, machineStop api.V1MachineStop) ([]byte, error) {
	stopEndpoint := fmt.Sprintf("/%s/stop", machineStop.ID)
	body, err := json.Marshal(machineStop)
	if err != nil {
		return nil, fmt.Errorf("failed to launch machine %s", err)
	}

	return f.sendRequest(ctx, nil, http.MethodPost, stopEndpoint, body)
}

func (f *Client) Get(ctx context.Context, machineID string) ([]byte, error) {
	getEndpoint := fmt.Sprintf("/%s", machineID)

	return f.sendRequest(ctx, nil, http.MethodGet, getEndpoint, nil)
}

func (f *Client) Kill(ctx context.Context, machineKillInput api.KillMachineInput) ([]byte, error) {
	killEndpoint := fmt.Sprintf("/%s?kill=%t", machineKillInput.ID, machineKillInput.Force)

	return f.sendRequest(ctx, nil, http.MethodDelete, killEndpoint, nil)
}

func (f *Client) sendRequest(ctx context.Context, machine *api.V1Machine, method, endpoint string, data []byte) ([]byte, error) {
	peerIP := f.peerIP
	if machine != nil {
		peerIP = resolvePeerIP(machine.PrivateIP)
	}

	targetEndpoint := fmt.Sprintf("http://[%s]:4280/v1/machines%s", peerIP, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, targetEndpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("could not create new request, %w", err)
	}
	req.SetBasicAuth(f.app.Name, f.authToken)

	logger.FromContext(ctx).Debugf("Running %s %s... ", method, endpoint)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body, %w", err)
	}

	return b, nil
}

func resolvePeerIP(ip string) string {
	peerIP := net.ParseIP(ip)
	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	return net.IP(natsIPBytes[:]).String()
}
