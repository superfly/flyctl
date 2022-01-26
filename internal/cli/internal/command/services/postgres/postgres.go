// Package postgres implements the postgres command chain.
package postgres

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// New initializes and returns a new postgres Command.
func New() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		long = `
`
		short = ""
	)

	cmd = command.New("postgres", short, long, nil)

	cmd.AddCommand(
		newLaunch(),
		newConnect(),
		newAttach(),
	)

	return
}
