package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("start", short, long, runStart,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}

func runStart(ctx context.Context) (err error) {
	if client, err := agent.DefaultClient(ctx); err == nil {
		_ = client.Kill(ctx)
	}

	apiClient := client.FromContext(ctx).API()
	if _, err := agent.Establish(ctx, apiClient); err != nil {
		return fmt.Errorf("failed starting agent: %w", err)
	}

	return nil
}
