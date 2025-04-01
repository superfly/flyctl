package mpg

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n"
	)

	cmd := command.New("managed-postgres", short, long, nil)

	cmd.Aliases = []string{"mpg"}

	cmd.AddCommand(
		newProxy(),
		newConnect(),
	)

	return cmd
}
