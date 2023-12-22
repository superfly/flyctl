package tigris

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Tigris object storage buckets"
		long  = short + "\n"
	)

	cmd = command.New("storage", short, long, nil)
	cmd.Aliases = []string{"tigris"}
	cmd.AddCommand(create(), list(), dashboard(), destroy())

	return cmd
}
