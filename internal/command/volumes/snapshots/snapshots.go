package snapshots

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage volume snapshots."

		long = short + " A snapshot is a point-in-time copy of a volume. Snapshots can be used to create new volumes or restore a volume to a previous state."

		usage = "snapshots"
	)

	snapshots := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	snapshots.Aliases = []string{"snapshot", "snaps"}

	snapshots.AddCommand(
		newList(),
		newCreate(),
	)

	return snapshots
}
