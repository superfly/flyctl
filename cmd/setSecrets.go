package cmd

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
)

// var appName string

func init() {
	secretsCmd.AddCommand(setSecretsCmd)

	setSecretsCmd.PersistentFlags().StringVarP(&appName, "app_name", "a", "", "fly app name")
}

var setSecretsCmd = &cobra.Command{
	Use: "set",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: func(cmd *cobra.Command, args []string) error {
		return validateArgs(args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {

		input := api.SetSecretsInput{AppID: appName}

		for _, pair := range args {
			parts := strings.Split(pair, "=")
			key := parts[0]
			value := parts[1]
			if value == "-" {
				inval, err := helpers.ReadStdin(4 * 1024)
				if err != nil {
					panic(err)
				}
				value = inval
			}

			input.Secrets = append(input.Secrets, api.SecretInput{Key: key, Value: value})
		}

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		req := client.NewRequest(`
		    mutation ($input: SetSecretsInput!) {
					setSecrets(input: $input) {
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

func validateArgs(args []string) error {
	if len(args) < 1 {
		return errors.New("Requires at least one SECRET=VALUE pair")
	}

	stdin := helpers.HasPipedStdin()
	for _, pair := range args {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			return fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", pair)
		}
		if parts[1] == "-" && !stdin {
			return fmt.Errorf("Secret `%s` expects standard input but none provided", parts[0])
		}
	}

	return nil
}
