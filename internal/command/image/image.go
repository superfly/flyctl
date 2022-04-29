package image

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage app image"
		long  = short + "\n"

		usage = "image"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Args = cobra.NoArgs

	cmd.Aliases = []string{"img"}

	cmd.AddCommand(
		newShow(),
		newUpdate(),
	)

	return cmd
}
