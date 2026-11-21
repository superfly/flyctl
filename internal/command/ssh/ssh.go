package ssh

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		short = `Use SSH to log into or run commands on Machines`
		long  = `Connect to your app's Machines over SSH through a WireGuard tunnel, which
starts the Fly agent if needed. ` + "`fly ssh console`" + ` opens a shell or runs a
command, and ` + "`fly ssh sftp`" + ` transfers files. flyctl issues a short-lived
SSH certificate for each connection; use ` + "`fly ssh issue`" + ` to get one for
your own SSH client.`
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
