package cmd

import (
	"github.com/spf13/cobra"
)

func newAppSecretsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "manage app secrets",
	}

	cmd.AddCommand(newAppSecretsListCommand())
	cmd.AddCommand(newAppSecretsSetCommand())
	cmd.AddCommand(newAppSecretsUnsetCommand())

	return cmd
}
