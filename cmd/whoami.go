package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
)

func init() {
	rootCmd.AddCommand(whoami)
}

func init() {
}

var whoami = &cobra.Command{
	Use: "whoami",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	RunE: func(cmd *cobra.Command, args []string) error {

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		query := `
			query {
				currentUser {
					email
				}
			}
		`

		req := client.NewRequest(query)

		data, err := client.Run(req)
		if err != nil {
			return err
		}

		fmt.Printf("Current user: %s\n", data.CurrentUser.Email)

		return nil
	},
}
