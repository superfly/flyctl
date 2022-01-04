package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStop() *cobra.Command {
	const (
		short = "Stop the Fly agent"
		long  = short + "\n"
	)

	return command.New("stop", short, long, runStop,
		command.RequireSession,
	)
}

func runStop(ctx context.Context) error {
	client, err := fetchClient(ctx)
	if err != nil {
		return err
	}

	if err := client.Kill(ctx); err != nil {
		return fmt.Errorf("failed killing agent: %w", err)
	}

	return nil
}
