package extensions

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newPlanetscale() (cmd *cobra.Command) {

	const (
		short = "Setup a PlanetScale project for this app"
		long  = short + "\n"
	)

	cmd = command.New("planetscale", short, long, nil)
	cmd.AddCommand(newPlanetscaleCreate())

	return cmd
}
