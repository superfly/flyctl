package machines

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long = `The machines command will show you how to interact with machines in fly.
`
		short = "Commands that manage machines"
		usage = "machines <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newList(),
	)

	return cmd
}
