package hosts

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Show hosts' incidents"
		long  = "Show hosts' incidents affecting applications"
	)

	cmd = command.New("hosts", short, long, nil)
	cmd.AddCommand(list())

	return cmd
}
