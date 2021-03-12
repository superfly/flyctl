package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/helpers"
)

func newSecretsCommand(client *client.Client) *Command {

	secretsStrings := docstrings.Get("secrets")
	cmd := BuildCommandKS(nil, nil, secretsStrings, client, requireSession, requireAppName)

	secretsListStrings := docstrings.Get("secrets.list")
	BuildCommandKS(cmd, runListSecrets, secretsListStrings, client, requireSession, requireAppName)

	secretsSetStrings := docstrings.Get("secrets.set")
	set := BuildCommandKS(cmd, runSetSecrets, secretsSetStrings, client, requireSession, requireAppName)

	//TODO: Move examples into docstrings
	set.Command.Example = `flyctl secrets set FLY_ENV=production LOG_LEVEL=info
	echo "long text..." | flyctl secrets set LONG_TEXT=-
	flyctl secrets set FROM_A_FILE=- < file.txt
	`
	set.Command.Args = cobra.MinimumNArgs(1)
	set.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})

	secretsImportStrings := docstrings.Get("secrets.import")
	importCmd := BuildCommandKS(cmd, runImportSecrets, secretsImportStrings, client, requireSession, requireAppName)
	importCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})

	secretsUnsetStrings := docstrings.Get("secrets.unset")
	unset := BuildCommandKS(cmd, runSecretsUnset, secretsUnsetStrings, client, requireSession, requireAppName)
	unset.Command.Args = cobra.MinimumNArgs(1)

	unset.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})

	return cmd
}

func runListSecrets(ctx *cmdctx.CmdContext) error {
	secrets, err := ctx.Client.API().GetAppSecrets(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Secrets{Secrets: secrets})
}

func runSetSecrets(cc *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	app, err := cc.Client.API().GetApp(cc.AppName)
	if err != nil {
		return err
	}

	secrets := make(map[string]string)

	for _, pair := range cc.Args {
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
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	release, err := cc.Client.API().SetSecrets(cc.AppName, secrets)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc)
}

func runImportSecrets(cc *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	app, err := cc.Client.API().GetApp(cc.AppName)
	if err != nil {
		return err
	}

	secrets := make(map[string]string)

	secretsString, err := ioutil.ReadAll(os.Stdin)

	if err != nil {
		return err
	}

	secretsArray := strings.Split(string(secretsString), "\n")

	parsestate := 0
	parsedkey := ""
	var parsebuffer strings.Builder

	for _, line := range secretsArray {
		switch parsestate {
		case 0:
			if line != "" {
				parts := strings.SplitN(line, "=", 2)
				if strings.HasPrefix(parts[1], "\"\"\"") {
					// Switch to multiline
					parsestate = 1
					parsedkey = parts[0]
					parsebuffer.WriteString(strings.TrimPrefix(parts[1], "\"\"\""))
					parsebuffer.WriteString("\n")
				} else {
					if len(parts) != 2 {
						return fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", line)
					}
					key := parts[0]
					value := parts[1]
					secrets[key] = value
				}
			}
		case 1:
			if strings.HasSuffix(line, "\"\"\"") {
				// End of multiline
				parsebuffer.WriteString(strings.TrimSuffix(line, "\"\"\""))
				secrets[parsedkey] = parsebuffer.String()
				parsebuffer.Reset()
				parsestate = 0
				parsedkey = ""
			} else {
				if line != "" {
					parsebuffer.WriteString(line)
				}
				parsebuffer.WriteString("\n")
			}

		}

	}

	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}
	fmt.Println(secrets)

	release, err := cc.Client.API().SetSecrets(cc.AppName, secrets)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc)
}

func runSecretsUnset(cc *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	app, err := cc.Client.API().GetApp(cc.AppName)
	if err != nil {
		return err
	}

	if len(cc.Args) == 0 {
		return errors.New("Requires at least one secret name")
	}

	release, err := cc.Client.API().UnsetSecrets(cc.AppName, cc.Args)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc)
}
