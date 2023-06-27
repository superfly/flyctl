package planetscale

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage PlanetScale MySQL databases"
		long  = short + "\n"
	)

	cmd = command.New("planetscale", short, long, nil)
	cmd.Aliases = []string{"mysql"}
	cmd.AddCommand(create(), destroy(), dashboard(), list(), status())

	return cmd
}
