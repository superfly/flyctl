package agent

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart the Fly agent"
		long  = short + "\n"
	)

	return command.New("restart", short, long, runStart,
		command.RequireSession,
	)
}
