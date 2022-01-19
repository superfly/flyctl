package image

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func New() *cobra.Command {
	const (
		long  = "Manage app image"
		short = "Manage app image"

		usage = "image"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Aliases = []string{"img"}

	cmd.AddCommand(
		newShow(),
		newUpdate(),
	)

	return cmd
}
