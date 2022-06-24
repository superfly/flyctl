package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new PostgreSQL cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, nil)

	return cmd
}
