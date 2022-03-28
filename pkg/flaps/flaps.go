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
	peerIP    string
	authToken string
}

func New(ctx context.Context, orgSlug string) (*Client, error) {
	client := client.FromContext(ctx).API()
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("ssh: can't build tunnel for %s: %s", orgSlug, err)
	}

	return &Client{
		peerIP:    resolvePeerIP(dialer.State().Peer.Peerip),
		authToken: flyctl.GetAPIToken(),
	}, nil
}

func (f *Client) Launch(ctx context.Context, builder api.LaunchMachineInput) ([]byte, error) {
	targetEndpoint := fmt.Sprintf("http://[%s]:4280/v1/machines", f.peerIP)

	body, err := json.Marshal(builder)
	if err != nil {
		return nil, err
	}

	return f.sendRequest(ctx, nil, http.MethodPost, targetEndpoint, body)
}

func (f *Client) Stop(ctx context.Context, machine *api.Machine) ([]byte, error) {
	stopEndpoint := fmt.Sprintf("/v1/machines/%s/stop", machine.ID)

	return f.sendRequest(ctx, machine, http.MethodPost, stopEndpoint, nil)
}

func (f *Client) Get(ctx context.Context, machine *api.Machine) ([]byte, error) {
	getEndpoint := fmt.Sprintf("/v1/machines/%s", machine.ID)

	return f.sendRequest(ctx, machine, http.MethodGet, getEndpoint, nil)
}

func (f *Client) sendRequest(ctx context.Context, machine *api.Machine, method, endpoint string, data []byte) ([]byte, error) {
	peerIP := f.peerIP
	if machine != nil {
		peerIP = resolvePeerIP(machine.IPs.Nodes[0].IP)
	}

	targetEndpoint := fmt.Sprintf("http://[%s]:4280%s", peerIP, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, targetEndpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(machine.App.ID, f.authToken)

	logger.FromContext(ctx).Debugf("Running %s %s... ", method, endpoint)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
