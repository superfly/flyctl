package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"

	"github.com/superfly/flyctl/internal/command"
)

func newStop() (cmd *cobra.Command) {
	const (
		short = "Stop the Fly agent"
		long  = `Stop the running Fly agent. Its open WireGuard tunnels close, which
interrupts any ` + "`fly proxy`" + ` or ` + "`fly ssh`" + ` sessions using it. Fails
if no agent is running. The next command that needs the agent starts a new
one.
`
	)

	cmd = command.New("stop", short, long, runStop)

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
