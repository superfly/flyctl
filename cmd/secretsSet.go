package cmd

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

// var appName string

func init() {
	secretsCmd.AddCommand(secretsSetCmd)
	addAppFlag(secretsSetCmd)
}

var secretsSetCmd = &cobra.Command{
	Use:   "set [flags] NAME=VALUE NAME=VALUE ...",
	Short: "set encrypted secrets",
	Long: `
Set one or more encrypted secrets for an app.

Secrets are provided to apps at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for 
the application and vm environment.

Any value that equals "-" will be assigned from STDIN instead of args.
`,
	Example: `flyctl secrets set FLY_ENV=production LOG_LEVEL=info
echo "long text..." | flyctl secrets set LONG_TEXT=-
flyctl secrets set FROM_A_FILE=- < file.txt
`,
	Args: func(cmd *cobra.Command, args []string) error {
		return validateArgs(args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := viper.GetString(flyctl.ConfigAppName)
		if appName == "" {
			return errors.New("No app provided")
		}

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

		query := `
			mutation ($input: SetSecretsInput!) {
				setSecrets(input: $input) {
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
