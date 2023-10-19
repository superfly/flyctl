package planetscale

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
		short = "Provision a PlanetScale MySQL database"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runPlanetscaleCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your database",
		},
	)
	return cmd
}

func runPlanetscaleCreate(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	extension, err := extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
		AppName:      appName,
		Provider:     "planetscale",
		Organization: org,
	})
	if err != nil {
		return err
	}

	return secrets.DeploySecrets(ctx, gql.ToAppCompact(extension.App), false, false)
}
