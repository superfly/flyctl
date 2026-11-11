package scale

import (
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Scale app resources"
		long  = `Change how many Machines an app runs and how big they are. Use
` + "`fly scale count`" + ` to add or remove Machines, ` + "`fly scale vm`" + ` and
` + "`fly scale memory`" + ` to resize them, and ` + "`fly scale show`" + ` to see the
current size and count of each process group.`
	)
	cmd := command.New("scale", short, long, nil)
	cmd.AddCommand(
		newScaleVm(),
		newScaleMemory(),
		newScaleShow(),
		newScaleCount(),
	)

	return cmd
}
