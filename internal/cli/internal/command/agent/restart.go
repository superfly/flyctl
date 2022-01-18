package agent

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRestart() (cmd *cobra.Command) {
	const (
		short = "Restart the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("restart", short, long, runStart,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}
