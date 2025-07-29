package enveloop

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/flag"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Provision and manage Enveloop projects"
		long  = short + "\n"
	)

	cmd = command.New("enveloop", short, long, nil)
	cmd.AddCommand(create(), list(), dashboard(), destroy(), status())

	return cmd
}

var SharedFlags = flag.Set{}
