// Package setup implements the setup command chain. fly setup agent gets the
// coding agents on this machine ready to work with Fly.io.
package setup

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns the setup command group.
func New() *cobra.Command {
	const (
		short = "Prepare this machine to work with Fly"
		long  = `Commands that prepare a local environment for Fly.

'fly setup agent' installs the Fly agent skills and the hosted MCP server
into the coding agents on this machine, and makes sure you are signed in.`
	)

	cmd := command.New("setup", short, long, nil)
	cmd.AddCommand(newAgent())

	return cmd
}
