package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart the Fly agent"
		long  = short + "\n"
	)

	return command.New("restart", short, long, runRestart,
		command.RequireSession,
	)
}

func runRestart(ctx context.Context) error {
	if client, err := fetchClient(ctx); err == nil {
		_ = client.Kill(ctx)
	}

	api := client.FromContext(ctx).API()
	if _, err := agent.Establish(ctx, api); err != nil {
		return fmt.Errorf("failed starting agent: %w", err)
	}

	return nil
}
