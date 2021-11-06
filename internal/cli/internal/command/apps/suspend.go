package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newSuspend() *cobra.Command {
	const (
		long = `The APPS SUSPEND command will suspend an application. 
All instances will be halted leaving the application running nowhere.
It will continue to consume networking resources (IP address). See APPS RESUME
for details on restarting it.
`

		short = "Suspend an application"

		usage = "suspend [APPNAME]"
	)

	suspend := command.New(usage, short, long, runSuspend,
		command.RequireSession)

	suspend.Args = cobra.RangeArgs(0, 1)

	return suspend
}

func runSuspend(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
