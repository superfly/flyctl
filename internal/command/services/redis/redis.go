// Package redis implements the redis command chain.
package redis

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new redis Command.
func New() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
`
		// TODO: document command
		short = ""
	)

	cmd = command.New("redis", short, long, nil)

	cmd.AddCommand(
		newLaunch(),
	)

	return
}
