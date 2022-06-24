package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newList() *cobra.Command {
	const (
		short = "List postgres clusters"
		long  = short + "\n"
	)

	cmd := command.New("list", short, long, nil)

	return cmd
}
