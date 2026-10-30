// Package setup implements `fly setup`, commands that prepare a machine for Fly.
package setup

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns the setup command group.
func New() *cobra.Command {
	const (
		short = "Prepare this machine to use Fly from a coding agent"
		long  = `Commands that prepare a local environment for Fly.io.

The agent subcommand installs the Fly agent skills and the hosted MCP server
into the coding agents detected on this machine.`
	)

	cmd := command.New("setup", short, long, nil)
	cmd.AddCommand(newAgent())

	return cmd
}
