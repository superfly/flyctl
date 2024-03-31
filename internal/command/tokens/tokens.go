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

	hiddenDeploy := newDeploy()
	hiddenDeploy.Hidden = true

	hiddenOrg := newOrg()
	hiddenOrg.Hidden = true

	cmd.AddCommand(
		newCreate(),
		newList(),
		newRevoke(),
		newAttenuate(),
		newDebug(),
		new3P(),
		hiddenDeploy,
		hiddenOrg,
	)

	return cmd
}
