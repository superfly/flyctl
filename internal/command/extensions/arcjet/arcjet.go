package arcjet

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Arcjet"
		long  = short + "\n"
	)

	cmd = command.New("arcjet", short, long, nil)
	cmd.AddCommand(create())
	cmd.AddCommand(dashboard())
	cmd.AddCommand(list())
	// TODO(danni-popova): Add remaining commands once functionality is enabled

	return cmd
}
