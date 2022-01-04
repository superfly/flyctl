// Package agent implements the agent command chain.
package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

// New initializes and returns a new agent Command.
func New() *cobra.Command {
	const (
		short = "Commands that manage the Fly agent"
		long  = short + "\n"
		usage = "agent <command>"
	)

	agent := command.New(usage, short, long, nil)

	agent.AddCommand(
		newPing(),
		newStart(),
		newStop(),
		newRestart(),
		newDaemonStart(),
	)

	return agent
}

func fetchClient(ctx context.Context) (c *agent.Client, err error) {
	client := client.FromContext(ctx).API()

	if c, err = agent.DefaultClient(client); err != nil {
		err = fmt.Errorf("failed initializing agent client: %w", err)
	}

	return
}
