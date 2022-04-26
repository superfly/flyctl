package builder

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long = `"Commands for managing remote builder"
`
		short = "Manage remote builder"
		usage = "builder"
	)

	builder := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	builder.AddCommand(
		newUpdate(),
		newShow(),
	)

	return builder
}
