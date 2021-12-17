// Package apps implements the apps command chain.
package builds

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// New initializes and returns a new apps Command.
func New() (cmd *cobra.Command) {
	const (
		long = `Build commands expose your local and remote builds.
The LIST command will list all builds along with their status.
`
		short = "Manage application builds"
	)

	cmd = command.New("builds", short, long, nil)

	cmd.AddCommand(
		newList(),
	)

	return
}
