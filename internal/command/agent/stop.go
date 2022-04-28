package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/internal/command"
)

func newStop() (cmd *cobra.Command) {
	const (
		short = "Stop the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("stop", short, long, runStop,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}

func runStop(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = dial(ctx); err != nil {
		return
	}

	if err = client.Kill(ctx); err != nil {
		err = fmt.Errorf("failed stopping agent: %w", err)
	}

	return
}
