// Package agent implements the agent command chain.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent/client"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/state"
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
		newRun(),
		newPing(),
		newStart(),
		newStop(),
		newRestart(),
	)

	return agent
}

func newClient(ctx context.Context) (c *client.Client, err error) {
	switch c, err = client.New(ctx, pathToSocket(ctx)); {
	case errors.Is(err, fs.ErrNotExist):
		err = errors.New("no agent is currently running")
	case err != nil:
		err = fmt.Errorf("failed initializing agent client: %w", err)
	}

	return
}

func pathToSocket(ctx context.Context) string {
	return pathToAgentFile(ctx, "agent.sock")
}

func pathToPID(ctx context.Context) string {
	return pathToAgentFile(ctx, "agent.pid")
}

func pathToAgentFile(ctx context.Context, name string) string {
	return filepath.Join(state.ConfigDirectory(ctx), name)
}
