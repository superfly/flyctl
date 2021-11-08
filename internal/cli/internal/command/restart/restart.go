package restart

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `The RESTART command will restart all running vms. 
`
		short = "Restart an application"
		usage = "restart [APPNAME]"
	)

	restart := command.New(usage, short, long, apps.RunRestart,
		command.RequireSession)

	restart.Args = cobra.ExactArgs(1)

	return restart
}
