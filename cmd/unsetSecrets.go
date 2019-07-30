package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
)

// var appName string

func init() {
	secretsCmd.AddCommand(unsetSecretsCmd)

	unsetSecretsCmd.PersistentFlags().StringVarP(&appName, "app_name", "a", "", "fly app name")
}

var unsetSecretsCmd = &cobra.Command{
	Use: "unset",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input := api.UnsetSecretsInput{AppID: appName, Keys: args}

		client, err := api.NewClient()
		if err != nil {
			return nil
		}

		req := client.NewRequest(`
		    mutation ($input: UnsetSecretsInput!) {
					unsetSecrets(input: $input) {
						deployment {
							id
							status
						}
					}
				}
		`)

		req.Var("input", input)

		data, err := client.Run(req)
		if err != nil {
			return err
		}

		log.Printf("%+v\n", data)

		return nil
	},
}
