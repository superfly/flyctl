package ssh

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long  = `Use SSH to login to or run commands on VMs`
		short = long
	)

	cmd := command.New("ssh", short, long, nil)

	cmd.AddCommand(
		newConsole(),
		// newEstablish(),
		// newIssue(),
		newLog(),
	)

	return cmd
}
