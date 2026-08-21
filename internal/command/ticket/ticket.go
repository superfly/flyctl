// Package ticket implements the ticket command chain.
package ticket

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Open support tickets with Fly.io"
		long  = `Open support tickets with the Fly.io support team from the command line.
Requires an organization with a support plan.`
	)
	cmd := command.New("ticket", short, long, nil)
	cmd.Aliases = []string{"mayday"}
	cmd.Hidden = true // TODO: unhide when the support API endpoint ships

	cmd.AddCommand(
		newCreate(),
	)

	return cmd
}
