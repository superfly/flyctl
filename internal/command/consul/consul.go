package consul

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Enable and manage Consul clusters"
		long  = "Connect an app to a Fly.io-managed Consul cluster. `fly consul attach`\n" +
			"stores the cluster URL as an app secret; `fly consul detach` removes it."
	)
	cmd := command.New("consul", short, long, nil)
	cmd.AddCommand(
		newAttach(),
		newDetach(),
	)

	return cmd
}
