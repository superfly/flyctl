package proxy

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Commands for proxying and interacting with Fly's proxy"
		long  = short + "\n"
		usage = "proxy <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(
		newStart(),
		newBalance(),
	)

	return cmd
}
