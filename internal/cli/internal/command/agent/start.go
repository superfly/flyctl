package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start the Fly agent"
		long  = short + "\n"
	)

	return command.New("start", short, long, runStart,
		command.RequireSession,
	)
}

func runStart(ctx context.Context) (err error) {
	if err = RunStop(ctx); err != nil {
		return
	}

	panic(err)
	return
}
