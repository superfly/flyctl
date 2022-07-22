package redis

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// TODO: make internal once the open command has been deprecated
func NewOpen() (cmd *cobra.Command) {
	const (
		long = `Launch and manage a Redis instance`

		short = long
	)

	cmd = command.New("redis", short, long, nil)

	cmd.AddCommand(
		newCreate(),
	)

	return

}
