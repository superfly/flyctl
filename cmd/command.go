package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"

	"github.com/spf13/cobra"
)

// RunFn - Run function for commands which takes a command context
type RunFn func(cmdContext *cmdctx.CmdContext) error

// Command - Wrapper for a cobra command
type Command struct {
	*cobra.Command
}

// AddCommand adds subcommands to this command
func (c *Command) AddCommand(commands ...*Command) {
	for _, cmd := range commands {
		c.Command.AddCommand(cmd.Command)
	}
}

func namespace(c *cobra.Command) string {
	parentName := flyctl.NSRoot
	if c.Parent() != nil {
		parentName = c.Parent().Name()
	}
	return parentName + "." + c.Name()
}

// Initializer - Retains Setup and PreRun functions
type Initializer struct {
	Setup  InitializerFn
	PreRun InitializerFn
}

// Option - A wrapper for an Initializer function that takes a command
type Option func(*Command) Initializer

// InitializerFn - A wrapper for an Initializer function that takes a command context
type InitializerFn func(*cmdctx.CmdContext) error
