package volumes

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long = "Commands for managing Fly Volumes associated with an application"

		short = "Volume management commands"
	)

	cmd := command.New("volumes [type] <name> [flags]", short, long, nil,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"vol"}

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDelete(),
		newSnapshot(),
		newShow(),
	)

	return cmd
}
