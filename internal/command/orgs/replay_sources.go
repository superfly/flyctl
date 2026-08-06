package orgs

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newReplaySources() *cobra.Command {
	const (
		long  = `Commands for managing cross-organization replay permissions.`
		short = "Manage allowed replay source organizations"
	)

	cmd := command.New("replay-sources", short, long, nil)

	cmd.AddCommand(
		newReplaySourcesList(),
		newReplaySourcesAdd(),
		newReplaySourcesRemove(),
	)

	return cmd
}
