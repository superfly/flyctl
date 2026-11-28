// Package agent implements the agent command chain.
package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
)

// New initializes and returns a new agent Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Manage the Fly agent, the background WireGuard process"
		long  = `The Fly agent is a local background process that holds the WireGuard
tunnels flyctl opens into your Fly.io private network. Commands such as
` + "`fly ssh console`" + `, ` + "`fly proxy`" + ` and ` + "`fly ping`" + ` use it, and
start it when it isn't running. It is unrelated to AI coding agents; for
those, see ` + "`fly setup agent`" + `.

You don't normally need these commands. Use them for debugging:
` + "`fly agent ping`" + ` checks the running agent and ` + "`fly agent restart`" + `
replaces it.
`
		usage = "agent <command>"
	)

	cmd = command.New(usage, short, long, nil)

	cmd.AddCommand(
		newRun(),
		newPing(),
		newStart(),
		newStop(),
		newRestart(),
	)

	if env.IsTruthy("DEV") {
		cmd.AddCommand(
			newResolve(),
			newProbe(),
			newInstances(),
			newEstablish(),
			newConnect(),
		)
	}

	return
}

func establish(ctx context.Context) (ac *agent.Client, err error) {
	client := flyutil.ClientFromContext(ctx)

	if ac, err = agent.Establish(ctx, client); err != nil {
		err = fmt.Errorf("failed establishing connection to agent: %w", err)
	}

	return
}

func dial(ctx context.Context) (client *agent.Client, err error) {
	if client, err = agent.DefaultClient(ctx); err != nil {
		err = fmt.Errorf("failed dialing agent: %w", err)
	}

	return
}

func socketPath(ctx context.Context) string {
	if socket := agent.SocketPathOverride(); socket != "" {
		return socket
	}

	return filepath.Join(state.ConfigDirectory(ctx), "fly-agent.sock")
}
