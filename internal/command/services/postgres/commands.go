package postgres

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

type postgresCmd struct {
	ctx    *context.Context
	app    *api.AppCompact
	dialer agent.Dialer
	io     *iostreams.IOStreams
}

func newPostgresCmd(ctx context.Context, app *api.AppCompact) (*postgresCmd, error) {
	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	return &postgresCmd{
		ctx:    &ctx,
		app:    app,
		dialer: dialer,
		io:     iostreams.FromContext(ctx),
	}, nil
}

func (pc *postgresCmd) updateSettings(config map[string]string) error {
	payload := updateRequest{PGParameters: config}
	configBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	subCmd := fmt.Sprintf("update --patch '%s'", string(configBytes))
	cmd := fmt.Sprintf("stolonctl-run %s", encodeCommand(subCmd))

	resp, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, nil, cmd)
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
