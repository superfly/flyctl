package tigris

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Supabase Postgresql databases"
		long  = short + "\n"
	)

	cmd = command.New("tigris", short, long, nil)
	cmd.AddCommand(create(), dashboard())

	return cmd
}
