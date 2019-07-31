package cmd

import (
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "manage authentication",
	}

	cmd.AddCommand(newAuthWhoamiCommand())
	cmd.AddCommand(newAuthLoginCommand())
	cmd.AddCommand(newAuthLogoutCommand())

	return cmd
}
