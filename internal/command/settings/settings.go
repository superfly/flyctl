package settings

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("settings", "Manage flyctl settings", "", nil)

	cmd.AddCommand(
		newAnalytics(),
		newAutoUpdate(),
		newSynthetics(),
	)

	return cmd
}
