package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = `Manage Postgre clusters.`

		long = short + "\n"
	)

	cmd := command.New("postgres", short, long, nil)

	cmd.Aliases = []string{"pg"}

	cmd.AddCommand(
		newAttach(),
		newConfig(),
		newConnect(),
		newCreate(),
		newDb(),
		newDetach(),
		newList(),
		newRestart(),
		newUsers(),
		newUpdate(),
	)

	return cmd
}
