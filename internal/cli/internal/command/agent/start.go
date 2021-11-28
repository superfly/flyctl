package agent

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStart() *cobra.Command {
	const (
		short = "Start the Fly agent"
		long  = short + "\n"
	)

	return command.New("start", short, long, runRestart,
		command.RequireSession,
	)
}
