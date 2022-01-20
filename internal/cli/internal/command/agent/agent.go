// Package agent implements the agent command chain.
package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	apiClient "github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/env"
)

// New initializes and returns a new agent Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Commands that manage the Fly agent"
		long  = short + "\n"
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

	if env.IsSet("DEV") {
		cmd.AddCommand(
			newResolve(),
			newProbe(),
		)
	}

	return
}

func establish(ctx context.Context) (client *agent.Client, err error) {
	apiClient := apiClient.FromContext(ctx).API()
	if client, err = agent.Establish(ctx, apiClient); err != nil {
		err = fmt.Errorf("failed establishing connection to agent: %w", err)
	}

	return
}

func dial(ctx context.Context) (client *agent.Client, err error) {
	if client, err = agent.NewClient(ctx, "unix", socketPath(ctx)); err != nil {
		err = fmt.Errorf("failed dialing agent: %w", err)
	}

	return
}

func socketPath(ctx context.Context) string {
	return filepath.Join(state.ConfigDirectory(ctx), "fly-agent.sock")
}
