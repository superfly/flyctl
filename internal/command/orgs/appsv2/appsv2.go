package appsv2

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	cmd := command.New(
		`apps-v2`,
		`Manage apps v2 default`,
		`Commands for managing apps v2 default on or off setting for the org`,
		nil,
		command.RequireSession,
	)
	cmd.AddCommand(
		newShow(),
		newDefaultOn(),
		newDefaultOff(),
	)
	return cmd
}
