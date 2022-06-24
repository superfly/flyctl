package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newDb() *cobra.Command {
	const (
		short = "manage databases in a clutser"
		long  = short + "\n"
	)

	cmd := command.New("db", short, long, nil)

	cmd.AddCommand()

	return cmd
}
