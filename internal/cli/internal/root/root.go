// Package root implements the root command.
package root

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/internal/cli/internal/version"
	"github.com/superfly/flyctl/internal/client"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	/*
		root := command.FromDocstrings("flyctl", nil)
		root.SilenceUsage = true
		root.SilenceErrors = true

		fs := root.PersistentFlags()

		_ = fs.StringP(flag.AccessTokenName, "t", "", "Fly API Access Token")
		_ = fs.BoolP(flag.JSONName, "j", false, "JSON output")
		_ = fs.BoolP(flag.VerboseName, "v", false, "Verbose output")

		root.AddCommand(
			apps.New(),
			version.New(),
			...
		)

		return root
	*/

	// what follows is a hack in order to achieve compatibility with what exists
	// already. the commented out code above, is what should remain after the
	// migration is complete.

	// newCommands is the set of commands which work with the new way
	newCommands := []*cobra.Command{
		version.New(),
	}

	// newCommandNames is the set of the names of the above commands
	newCommandNames := make(map[string]struct{}, len(newCommands))
	for _, cmd := range newCommands {
		newCommandNames[cmd.Name()] = struct{}{}
	}

	// instead of root being constructed like in the commented out snippet, we
	// rebuild it the old way.
	root := cmd.NewRootCmd(client.New())

	// gather the slice of commands which must be replaced with their new
	// iterations
	var commandsToReplace []*cobra.Command
	for _, cmd := range root.Commands() {
		if _, exists := newCommandNames[cmd.Name()]; exists {
			commandsToReplace = append(commandsToReplace, cmd)
		}
	}

	// remove them
	root.RemoveCommand(commandsToReplace...)

	// and finally, add the new iterations
	root.AddCommand(newCommands...)

	return root
}
