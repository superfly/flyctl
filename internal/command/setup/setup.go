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
		long  = `Prepare this computer so AI coding agents (such as Claude Code, Codex or
Cursor) can work with Fly.io.

` + "`fly setup agent`" + ` installs the Fly agent skills and adds the hosted Fly
MCP server to every coding agent it detects. Here "agent" means an AI coding
agent, not the background WireGuard process managed by ` + "`fly agent`" + `.`
	)

	cmd := command.New("setup", short, long, nil)
	cmd.AddCommand(newAgent())

	return cmd
}
