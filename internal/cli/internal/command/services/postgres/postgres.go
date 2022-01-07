// Package postgres implements the postgres command chain.
package postgres

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// New initializes and returns a new postgres Command.
func New() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
`
		// TODO: document command
		short = ""
	)

	cmd = command.New("postgres", short, long, nil)

	cmd.AddCommand(
		newLaunch(),
	)

	return
}
