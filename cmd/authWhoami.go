package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
)

func newAuthWhoamiCommand() *cobra.Command {
	whoami := &authWhoamiCommand{}

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "print the currently authenticated user",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return whoami.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return whoami.Run(args)
		},
	}

	return cmd
}

type authWhoamiCommand struct {
	client *api.Client
}

func (cmd *authWhoamiCommand) Init() error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.client = client

	return nil
}

func (cmd *authWhoamiCommand) Run(args []string) error {
	query := `
			query {
				currentUser {
					email
				}
			}
		`

	req := cmd.client.NewRequest(query)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	fmt.Printf("Current user: %s\n", data.CurrentUser.Email)

	return nil
}
