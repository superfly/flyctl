package arcjet

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {
	const (
		short = "Create an Arcjet site"
		long  = `Create an Arcjet site. In an app directory, or with ` + "`--app`" + `, the site is
attached to that app, and the command fails if the app already has one.
Otherwise the site belongs to an organization, which you pick from a
list unless you pass ` + "`--org`" + `.

The first time you use Arcjet you must accept its terms of service;
` + "`--yes`" + ` accepts them, and is required to do so without a terminal.

For an app's site, the values Arcjet provides are set as secrets on the
app and its machines restart to pick them up. If any of those secrets
already exist, none are set, and the values are printed for you to set
yourself.`
	)

	cmd = command.New("create",
		short,
		long,
		runCreate,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		extensions_core.SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your application",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	params := extensions_core.ExtensionParams{}

	if appName != "" {
		params.AppName = appName
	} else {
		org, err := orgs.OrgFromFlagOrSelect(ctx)

		if err != nil {
			return err
		}

		params.Organization = org
	}

	params.Provider = "arcjet"
	extension, err := extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, extension.App.Name, secrets.DeploymentArgs{
			Stage:    false,
			Detach:   false,
			CheckDNS: true,
		})
	}

	return
}
