package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"

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

func runListSecrets(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	secrets, err := cmdCtx.Client.API().GetAppSecrets(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	return cmdCtx.Render(&presenters.Secrets{Secrets: secrets})
}

func runSetSecrets(cc *cmdctx.CmdContext) error {
	ctx := cc.Command.Context()

	app, err := cc.Client.API().GetApp(ctx, cc.AppName)
	if err != nil {
		return err
	}

	secrets, err := cmdutil.ParseKVStringsToMap(cc.Args)
	if err != nil {
		return err
	}

	for k, v := range secrets {
		if v == "-" {
			if !helpers.HasPipedStdin() {
				return fmt.Errorf("Secret `%s` expects standard input but none provided", k)
			}
			inval, err := helpers.ReadStdin(64 * 1024)
			if err != nil {
				return fmt.Errorf("Error reading stdin for '%s': %s", k, err)
			}
			secrets[k] = inval
		}
	}

	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	release, err := cc.Client.API().SetSecrets(ctx, cc.AppName, secrets)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc, release.EvaluationID)
}

func runImportSecrets(cc *cmdctx.CmdContext) error {
	ctx := cc.Command.Context()

	app, err := cc.Client.API().GetApp(ctx, cc.AppName)
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

	release, err := cc.Client.API().SetSecrets(ctx, cc.AppName, secrets)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc, release.EvaluationID)
}

func runSecretsUnset(cc *cmdctx.CmdContext) error {
	ctx := cc.Command.Context()

	app, err := cc.Client.API().GetApp(ctx, cc.AppName)
	if err != nil {
		return err
	}

	if len(cc.Args) == 0 {
		return errors.New("Requires at least one secret name")
	}

	release, err := cc.Client.API().UnsetSecrets(ctx, cc.AppName, cc.Args)
	if err != nil {
		return err
	}

	if !app.Deployed {
		cc.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		return nil
	}

	cc.Statusf("secrets", cmdctx.SINFO, "Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc, release.EvaluationID)
}
