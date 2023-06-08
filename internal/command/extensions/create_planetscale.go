package extensions

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newPlanetscaleCreate() (cmd *cobra.Command) {

	const (
		short = "Provision a PlanetScale project for a Fly.io app"
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
			Description: "The name of your Redis database",
		},
	)
	return cmd
}

func runPlanetscaleCreate(ctx context.Context) (err error) {
	_, err = ProvisionExtension(ctx, "planetscale")

	return
}
