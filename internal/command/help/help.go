package help

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/olekukonko/tablewriter"
)

func New(root *cobra.Command) *cobra.Command {
	cmd := command.New("help", "Help on flyctl commands", "", Help(root))

	cmd.AddCommand(command.New("commands", "All flyctl commands", "", HelpCommands(root)))

	return cmd
}

// the output of `flyctl`, run by itself with no args
func NewRootHelp() *cobra.Command {
	return command.New("", "", "", func(ctx context.Context) error {
		fmt.Println(docstrings.Get("flyctl").Long)
		return nil
	})
}

// the output of `flyctl help`, possibly with more arguments
func Help(root *cobra.Command) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if cmd, _, err := root.Find(flag.Args(ctx)); err == nil && cmd != root {
			return cmd.Help()
		}

		fmt.Printf("this is help\n")
		return nil
	}
}

// the output of `flyctl help commands`; the master list of commands
func HelpCommands(root *cobra.Command) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		for _, cmd := range root.Commands() {
			fmt.Printf("%s %s\n", tablewriter.PadRight(cmd.Name(), " ", 15), cmd.Short)
		}

		return nil
	}
}
