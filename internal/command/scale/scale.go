package scale

import (
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Scale app resources"
		long  = `Scale application resources`
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
