package upstash

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
		short = "Provision an Upstash Redis database"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		flag.ReplicaRegions(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Redis database",
		},
		flag.Bool{
			Name:        "no-replicas",
			Description: "Don't prompt for selecting replica regions",
		},
		flag.Bool{
			Name:        "enable-eviction",
			Description: "Evict objects when memory is full",
		},
		flag.Bool{
			Name:        "disable-eviction",
			Description: "Disallow writes when the max data size limit has been reached",
		},
		flag.String{
			Name:        "plan",
			Description: "Upstash Redis plan",
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

	params.Provider = "upstash_redis"

	extension, err := extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	secrets.DeploySecrets(ctx, gql.ToAppCompact(extension.App), false, false)

	return
}
