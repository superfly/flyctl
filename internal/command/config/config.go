package config

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new platform Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Manage an app's configuration"
		long  = `Work with an app's configuration, the settings kept in fly.toml.
` + "`fly config show`" + ` prints the deployed configuration, ` + "`fly config save`" + `
writes it to a local file, and ` + "`fly config validate`" + ` checks a config file
for errors.`
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
