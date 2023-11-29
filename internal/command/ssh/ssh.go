package ssh

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long  = `Use SSH to log into or run commands on Machines`
		short = long
	)

	cmd := command.New("ssh", short, long, nil)

	cmd.AddCommand(
		newConsole(),
		newIssue(),
		newLog(),
		NewSFTP(),
	)

	return cmd
}
