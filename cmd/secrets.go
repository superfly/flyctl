package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
)

func newAppSecretsCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "secrets",
			Short: "manage app secrets",
		},
	}

	list := BuildCommand(runListSecrets, "list", "list app secret names", os.Stdout, true, requireAppName)

	setUse := "set [flags] NAME=VALUE NAME=VALUE ..."
	setHelp := `
Set one or more encrypted secrets for an app.

Secrets are provided to apps at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for 
the application and vm environment.

Any value that equals "-" will be assigned from STDIN instead of args.
`

	set := BuildCommand(runSetSecrets, setUse, setHelp, os.Stdout, true, requireAppName)
	set.Command.Example = `flyctl secrets set FLY_ENV=production LOG_LEVEL=info
	echo "long text..." | flyctl secrets set LONG_TEXT=-
	flyctl secrets set FROM_A_FILE=- < file.txt
	`
	set.Command.Args = cobra.MinimumNArgs(1)

	unset := BuildCommand(runSecretsUnset, "unset [flags] NAME NAME ...", "remove encrypted secrets", os.Stdout, true, requireAppName)
	unset.Command.Args = cobra.MinimumNArgs(1)

	cmd.AddCommand(list, set, unset)

	return cmd
}

func runListSecrets(ctx *CmdContext) error {
	query := `
			query ($appName: String!) {
				app(id: $appName) {
					secrets
				}
			}
		`

	req := ctx.FlyClient.NewRequest(query)

	req.Var("appName", ctx.AppName())

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name"})

	for _, secret := range data.App.Secrets {
		table.Append([]string{secret})
	}

	table.Render()

	return nil
}

func runSetSecrets(ctx *CmdContext) error {
	secrets := make(map[string]string)

	for _, pair := range ctx.Args {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			return fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", pair)
		}
		key := parts[0]
		value := parts[1]
		if value == "-" {
			if !helpers.HasPipedStdin() {
				return fmt.Errorf("Secret `%s` expects standard input but none provided", parts[0])
			}
			inval, err := helpers.ReadStdin(4 * 1024)
			if err != nil {
				return fmt.Errorf("Error reading stdin for '%s': %s", parts[0], err)
			}
			value = inval
		}

		if value == "" {
			return fmt.Errorf("Secret `%s` is empty", parts[0])
		}

		secrets[key] = value
	}

	if len(secrets) < 1 {
		return errors.New("Requires at least one SECRET=VALUE pair")
	}

	input := api.SetSecretsInput{AppID: ctx.AppName()}
	for Key, Value := range secrets {
		input.Secrets = append(input.Secrets, api.SecretInput{Key, Value})
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

	req := ctx.FlyClient.NewRequest(query)

	req.Var("input", input)

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", data)

	return nil
}

func runSecretsUnset(ctx *CmdContext) error {
	if len(ctx.Args) == 0 {
		return errors.New("Requires at least one secret name")
	}

	input := api.UnsetSecretsInput{AppID: ctx.AppName(), Keys: ctx.Args}

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

	req := ctx.FlyClient.NewRequest(query)
	req.Var("input", input)

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", data)

	return nil
}
