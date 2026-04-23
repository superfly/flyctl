package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new Managed Postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres cluster",
		},
		flag.String{
			Name:        "plan",
			Description: "The plan to use for the Postgres cluster (development, production, etc)",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
			Default:     10,
		},
		flag.Bool{
			Name:        "v2",
			Description: "Use MPG v2",
			Default:     false,
		},
		flag.Bool{
			Name:        "enable-postgis-support",
			Description: "Enable PostGIS for the Postgres cluster",
			Default:     false,
		},
		flag.Int{
			Name:        "pg-major-version",
			Description: "The major version of Postgres to use for the Postgres cluster. Supported versions are 16 and 17.",
			Default:     16,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
	var (
		appName = flag.GetString(ctx, "name")
		err     error
	)

	if appName == "" {
		// If no name is provided, try to get the app name from context
		if appName = appconfig.NameFromContext(ctx); appName != "" {
			// If we have an app name, use it to create a default database name
			appName = appName + "-db"
		} else {
			// If no app name is available, prompt for a name
			appName, err = prompt.SelectAppNameWithMsg(ctx, "Choose a database name:")
			if err != nil {
				return err
			}
		}
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "v2") {
		return cmdv2.RunCreate(ctx, org, appName)
	}

	return cmdv1.RunCreate(ctx, org, appName)

}
