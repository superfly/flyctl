package arcjet

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Arcjet sites"
		long  = `Provision and manage Arcjet sites for your Fly apps. Create one with
` + "`fly ext arcjet create`" + `, check it with ` + "`fly ext arcjet status`" + `, and open
Arcjet's console with ` + "`fly ext arcjet dashboard`" + `.`
	)

	cmd = command.New("arcjet", short, long, nil)
	cmd.AddCommand(create())
	cmd.AddCommand(dashboard())
	cmd.AddCommand(list())
	cmd.AddCommand(destroy())
	cmd.AddCommand(status())

	return cmd
}
