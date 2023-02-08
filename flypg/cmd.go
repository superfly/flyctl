package flypg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/iostreams"
)

type updateRequest struct {
	PGParameters map[string]string `json:"pgParameters,omitempty"`
}

type commandResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type Command struct {
	ctx    context.Context
	app    *api.AppCompact
	dialer agent.Dialer
	io     *iostreams.IOStreams
}

func NewCommand(ctx context.Context, app *api.AppCompact) (*Command, error) {
	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	return &Command{
		ctx:    ctx,
		app:    app,
		dialer: dialer,
		io:     iostreams.FromContext(ctx),
	}, nil
}

func (pc *Command) UpdateSettings(ctx context.Context, leaderIp string, config map[string]string) error {
	payload := updateRequest{PGParameters: config}
	configBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	subCmd := fmt.Sprintf("update --patch '%s'", string(configBytes))
	cmd := fmt.Sprintf("stolonctl-run %s", encodeCommand(subCmd))

	resp, err := ssh.RunSSHCommand(ctx, pc.app, pc.dialer, leaderIp, cmd, "root")
	if err != nil {
		return err
	}

	var result commandResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf(result.Message)
	}

	return nil
}

func (pc *Command) UnregisterMember(ctx context.Context, leaderIP string, standbyIP string) error {
	payload := encodeCommand(standbyIP)
	cmd := fmt.Sprintf("pg_unregister %s", payload)

	resp, err := ssh.RunSSHCommand(ctx, pc.app, pc.dialer, leaderIP, cmd, "root")
	if err != nil {
		return err
	}

	var result commandResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf(result.Message)
	}

	return nil
}

// encodeCommand will base64 encode a command string so it can be passed
// in with  exec.Command.
func encodeCommand(command string) string {
	return base64.StdEncoding.Strict().EncodeToString([]byte(command))
}
