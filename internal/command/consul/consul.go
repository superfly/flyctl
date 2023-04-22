package consul

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Enable and manage Consul clusters"
		long  = "Enable and manage Consul clusters"
	)
	cmd := command.New("consul", short, long, nil)
	cmd.AddCommand(
		newAttach(),
		newDetach(),
	)
	return cmd
}
