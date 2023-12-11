// Package agent implements the agent command chain.
package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/state"
)

// New initializes and returns a new agent Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Commands that manage the Fly agent, a background process that manages flyctl wireguard connections"
		long  = "The Fly agent is a background process that manages wireguard connections started by flyctl.\nCommands such as 'fly ssh' and 'fly proxy' use this agent.\nGenerally, the agent starts and stops automatically. These commands are useful for debugging.\n"
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
	client := client.FromContext(ctx).API()

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
	return filepath.Join(state.RuntimeDirectory(ctx), "fly-agent.sock")
}
