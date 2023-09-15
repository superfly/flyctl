package upstash

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Upstash Redis databases"
		long  = short + "\n"
	)

	cmd = command.New("upstash", short, long, nil)
	cmd.Aliases = []string{"redis"}
	cmd.AddCommand(create(), destroy(), dashboard(), list(), status())

	return cmd
}
