package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flyctl"
)

func newAuthLogoutCommand() *cobra.Command {
	logout := &authLogoutCommand{}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "destroy a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logout.Run(args)
		},
	}

	return cmd
}

type authLogoutCommand struct {
}

func (cmd *authLogoutCommand) Run(args []string) error {
	if err := flyctl.ClearSavedAccessToken(); err != nil {
		return err
	}

	fmt.Println("Session removed")

	return nil
}
