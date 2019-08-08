package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/helpers"
)

func newAppSecretsCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "secrets",
			Short: "manage app secrets",
		},
	}

	BuildCommand(cmd, runListSecrets, "list", "list app secrets", os.Stdout, true, requireAppName)

	setUse := "set [flags] NAME=VALUE NAME=VALUE ..."
	setHelp := `
Set one or more encrypted secrets for an app.

Secrets are provided to apps at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for 
the application and vm environment.

Any value that equals "-" will be assigned from STDIN instead of args.
`

	set := BuildCommand(cmd, runSetSecrets, setUse, setHelp, os.Stdout, true, requireAppName)
	set.Command.Example = `flyctl secrets set FLY_ENV=production LOG_LEVEL=info
	echo "long text..." | flyctl secrets set LONG_TEXT=-
	flyctl secrets set FROM_A_FILE=- < file.txt
	`
	set.Command.Args = cobra.MinimumNArgs(1)

	unset := BuildCommand(cmd, runSecretsUnset, "unset [flags] NAME NAME ...", "remove encrypted secrets", os.Stdout, true, requireAppName)
	unset.Command.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runListSecrets(ctx *CmdContext) error {
	fmt.Println(ctx.AppName())
	secrets, err := ctx.FlyClient.GetAppSecrets(ctx.AppName())
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Secrets{Secrets: secrets})
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

	release, err := ctx.FlyClient.SetSecrets(ctx.AppName(), secrets)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Release: release})
}

func runSecretsUnset(ctx *CmdContext) error {
	if len(ctx.Args) == 0 {
		return errors.New("Requires at least one secret name")
	}

	release, err := ctx.FlyClient.UnsetSecrets(ctx.AppName(), ctx.Args)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Release: release})
}
