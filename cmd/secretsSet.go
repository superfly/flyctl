package cmd

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

func newAppSecretsSetCommand() *cobra.Command {
	setSecrets := &appSecretsSetCommand{}

	cmd := &cobra.Command{
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
			return setSecrets.ValidateArgs(args)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return setSecrets.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return setSecrets.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&setSecrets.appName, "app", "a", "", `the app name to use`)

	return cmd
}

type appSecretsSetCommand struct {
	client  *api.Client
	appName string
}

func (cmd *appSecretsSetCommand) ValidateArgs(args []string) error {
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

func (cmd *appSecretsSetCommand) Init(args []string) error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.client = client

	if cmd.appName == "" {
		cmd.appName = flyctl.CurrentAppName()
	}
	if cmd.appName == "" {
		return fmt.Errorf("no app specified")
	}

	return nil
}

func (cmd *appSecretsSetCommand) Run(args []string) error {
	input := api.SetSecretsInput{AppID: cmd.appName}

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

	req := cmd.client.NewRequest(query)

	req.Var("input", input)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", data)

	return nil
}
