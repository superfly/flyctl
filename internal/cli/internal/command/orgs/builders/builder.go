package builders

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long = `"Commands for managing remote builders"
`
		short = "Manage remote builders"
		usage = "builders"
	)

	builder := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	builder.AddCommand(
		newUpdate(),
	)

	return builder
}
