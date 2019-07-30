package cmd

import (
	"errors"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

// var appName string

func init() {
	secretsCmd.AddCommand(unsetSecretsCmd)
	addAppFlag(unsetSecretsCmd)
}

var unsetSecretsCmd = &cobra.Command{
	Use: "unset",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := viper.GetString(flyctl.ConfigAppName)
		if appName == "" {
			return errors.New("No app provided")
		}

		input := api.UnsetSecretsInput{AppID: appName, Keys: args}

		client, err := api.NewClient()
		if err != nil {
			return nil
		}

		query := `
			mutation ($input: UnsetSecretsInput!) {
				unsetSecrets(input: $input) {
					deployment {
						id
						status
					}
				}
			}
		`

		req := client.NewRequest(query)
		req.Var("input", input)

		data, err := client.Run(req)
		if err != nil {
			return err
		}

		log.Printf("%+v\n", data)

		return nil
	},
}
