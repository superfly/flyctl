// Package root implements the root command.
package root

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
	"github.com/superfly/flyctl/internal/cli/internal/command/version"
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

	flyctl.InitConfig()

	// what follows is a hack in order to achieve compatibility with what exists
	// already. the commented out code above, is what should remain after the
	// migration is complete.

	// newCommands is the set of commands which work with the new way
	newCommands := []*cobra.Command{
		version.New(),
		apps.New(),
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

	// make sure the remaining old commands run the preparers
	// TODO: remove when migration is done
	wrapRunE(root)

	// and finally, add the new commands
	root.AddCommand(newCommands...)

	return root
}

func wrapRunE(cmd *cobra.Command) {
	if cmd.HasAvailableSubCommands() {
		for _, c := range cmd.Commands() {
			wrapRunE(c)
		}
	}

	if cmd.RunE == nil && cmd.Run == nil {
		return
	}

	if cmd.RunE == nil {
		panic(cmd.Name())
	}

	cmd.RunE = command.WrapRunE(cmd.RunE)
}
