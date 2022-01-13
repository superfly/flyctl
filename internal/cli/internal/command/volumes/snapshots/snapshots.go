package snapshots

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long = `"Commands for managing volume snapshots"
`
		short = "Manage volume snapshots"
		usage = "snapshots"
	)

	snapshots := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	snapshots.Aliases = []string{"snaps"}

	snapshots.AddCommand(
		newList(),
	)

	return snapshots
}
