package wafris

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {
	const (
		short = "Provision a Wafris WAF"
		long  = `Provision a Wafris WAF for an app. Run the command in an app directory
or pass ` + "`--app`" + `; it fails if the app already has a Wafris WAF.

The first time you use Wafris you must accept its terms of service;
` + "`--yes`" + ` accepts them, and is required to do so without a terminal.

The values Wafris provides are set as secrets on the app and its
machines restart to pick them up. If any of those secrets already exist,
none are set, and the values are printed for you to set yourself.`
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		extensions_core.SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your WAF",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	extension, err := extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
		AppName:  appName,
		Provider: "wafris",
	})

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, extension.App.Name, secrets.DeploymentArgs{
			Stage:    false,
			Detach:   false,
			CheckDNS: true,
		})
	}

	return err
}
