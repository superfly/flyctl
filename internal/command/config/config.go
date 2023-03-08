package config

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new platform Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Manage an app's configuration"
		long  = `The CONFIG commands allow you to work with an application's configuration.`
	)
	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newShow(),
		newSave(),
		newValidate(),
		newEnv(),
	)
	return
}
