package hosts

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Show host issues affecting an app"
		long  = `See issues on the hosts that run an app's Machines. Run
` + "`fly incidents hosts list`" + ` to list them.`
	)

	cmd = command.New("hosts", short, long, nil)
	cmd.AddCommand(list())

	return cmd
}
