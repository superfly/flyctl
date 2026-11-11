package snapshots

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage volume snapshots"

		long = `List and take snapshots of Fly Volumes. A snapshot is a point-in-time
copy of a volume. To restore data, create a new volume from a snapshot
with ` + "`fly volumes create --snapshot-id`" + `. See
` + "`fly volumes snapshots list`" + ` to find snapshot IDs.`

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
