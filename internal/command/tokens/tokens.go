package tokens

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage Fly.io API tokens"
		long  = "Manage Fly.io API tokens"
		usage = "tokens"
	)

	cmd := command.New(usage, short, long, nil)

	hiddenAliases := []*cobra.Command{
		newDeploy(),
		newLogs(),
	}
	for _, ha := range hiddenAliases {
		ha.Hidden = true
	}

	cmd.AddCommand(append(hiddenAliases, newCreate())...)

	return cmd
}
