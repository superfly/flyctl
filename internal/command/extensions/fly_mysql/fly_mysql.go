package fly_mysql

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage MySQL databases"
		long  = short + "\n"
	)

	cmd = command.New("mysql", short, long, nil)
	cmd.AddCommand(create(), list(), status(), destroy())

	return cmd
}
