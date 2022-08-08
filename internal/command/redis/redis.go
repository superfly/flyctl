package redis

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// TODO: make internal once the open command has been deprecated
func New() (cmd *cobra.Command) {
	const (
		long  = `Launch and manage an Upstash Redis instance`
		short = long
	)

	cmd = command.New("redis", short, long, nil)

	// To be exposed once Redis instances are being deployed inside Fly
	cmd.Hidden = true

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDelete(),
		newStatus(),
		newPlans(),
	)

	return cmd
}
