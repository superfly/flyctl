package image

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage app image"
		long  = `Inspect and update the image an app's Machines run. ` + "`fly image show`" + ` lists
each Machine's image and any newer version available; ` + "`fly image update`" + `
moves Machines to the latest version.`

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
