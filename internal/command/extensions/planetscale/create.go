package planetscale

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a PlanetScale MySQL database"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runPlanetscaleCreate, command.RequireSession, command.RequireAppName)
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

func runPlanetscaleCreate(ctx context.Context) (err error) {
	_, err = extensions_core.ProvisionExtension(ctx, "planetscale")

	return
}
