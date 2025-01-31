package supabase

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Supabase Postgres databases"
		long  = short + "\n"
	)

	cmd = command.New("supabase", short, long, nil)
	cmd.AddCommand(destroy(), dashboard(), list(), status())

	return cmd
}
