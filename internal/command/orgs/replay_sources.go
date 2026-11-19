package orgs

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newReplaySources() *cobra.Command {
	const (
		long = `Manage which other organizations may replay requests to apps in an
organization.

See the current list with ` + "`fly orgs replay-sources list`" + `, and change it
with ` + "`add`" + ` and ` + "`remove`" + `.`
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
