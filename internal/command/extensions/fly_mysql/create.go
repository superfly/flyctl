package fly_mysql

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a MySQL database"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		extensions_core.SharedFlags,
		SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your database",
		},
		flag.Int{
			Name:        "disk",
			Description: "Disk size (in GB) assigned to each cluster member",
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

	params.Provider = "fly_mysql"

	options := gql.AddOnOptions{}

	params.Options = optionsFromFlags(ctx, options)

	extension, err := extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), false, false)
	}

	return
}
