package redis

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// TODO: make internal once the open command has been deprecated
func New() (cmd *cobra.Command) {
	const (
		long  = `Launch and manage Redis databases managed by Upstash.com`
		short = long
	)

	cmd = command.New("redis", short, long, nil)

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDelete(),
		newStatus(),
		newPlans(),
		newUpdate(),
		newConnect(),
		newDashboard(),
		newReset(),
	)

	return cmd
}
