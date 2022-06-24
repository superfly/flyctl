package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newUsers() *cobra.Command {
	const (
		short = "Manage users in a postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("users", short, long, nil)

	cmd.AddCommand()

	return cmd
}
