package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRestart() *cobra.Command {
	const (
		long = `The APPS RESTART command will restart all running vms. 
`

		short = "Restart an application"

		usage = "restart [APPNAME]"
	)

	restart := command.New(usage, short, long, runRestart,
		command.RequireSession)

	restart.Args = cobra.RangeArgs(0, 1)

	return restart
}

func runRestart(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
