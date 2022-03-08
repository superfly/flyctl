package suspend

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `The SUSPEND command will suspend an application.
All instances will be halted leaving the application running nowhere.
It will continue to consume networking resources (IP address). See APPS RESUME
for details on restarting it.
`
		short = "Suspend an application"
		usage = "suspend [APPNAME]"
	)

	suspend := command.New(usage, short, long, apps.RunSuspend,
		command.RequireSession)

	return suspend
}
