package vm

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Commands that manage VM instances"
		long  = short + "\n"
		usage = "vm <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newStatus(),
		newStop(),
		newRestart(),
	)

	return cmd
}
