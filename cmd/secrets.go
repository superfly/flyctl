package cmd

import (
	"errors"
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/helpers"
)

func newAppSecretsCommand() *Command {

	secretsStrings := docstrings.Get("secrets")
	cmd := &Command{
		Command: &cobra.Command{
			Use:   secretsStrings.Usage,
			Short: secretsStrings.Short,
			Long:  secretsStrings.Long,
		},
	}

	secretsListStrings := docstrings.Get("secrets.list")
	BuildCommand(cmd, runListSecrets, secretsListStrings.Usage, secretsListStrings.Short, secretsListStrings.Long, true, os.Stdout, requireAppName)

	secretsSetStrings := docstrings.Get("secrets.set")
	set := BuildCommand(cmd, runSetSecrets, secretsSetStrings.Usage, secretsSetStrings.Short, secretsSetStrings.Long, true, os.Stdout, requireAppName)

	//TODO: Move examples into docstrings
	set.Command.Example = `flyctl secrets set FLY_ENV=production LOG_LEVEL=info
	echo "long text..." | flyctl secrets set LONG_TEXT=-
	flyctl secrets set FROM_A_FILE=- < file.txt
	`
	set.Command.Args = cobra.MinimumNArgs(1)

	secretsUnsetStrings := docstrings.Get("secrets.unset")
	unset := BuildCommand(cmd, runSecretsUnset, secretsUnsetStrings.Usage, secretsUnsetStrings.Short, secretsUnsetStrings.Long, true, os.Stdout, requireAppName)
	unset.Command.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runListSecrets(ctx *CmdContext) error {
	secrets, err := ctx.FlyClient.GetAppSecrets(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Secrets{Secrets: secrets})
}

func runSetSecrets(ctx *CmdContext) error {
	secrets := make(map[string]string)

	for _, pair := range ctx.Args {
		parts := strings.SplitN(pair, "=", 2)
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

		secrets[key] = value
	}

	if len(secrets) < 1 {
		return errors.New("Requires at least one SECRET=VALUE pair")
	}

	release, err := ctx.FlyClient.SetSecrets(ctx.AppName, secrets)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Release: release})
}

func runSecretsUnset(ctx *CmdContext) error {
	if len(ctx.Args) == 0 {
		return errors.New("Requires at least one secret name")
	}

	release, err := ctx.FlyClient.UnsetSecrets(ctx.AppName, ctx.Args)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Release: release})
}
