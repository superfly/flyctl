package volumes

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newSnapshot() *cobra.Command {
	const (
		long  = "Create a snapshot of a volume."
		short = "Create a snapshot of a volume."
	)

	cmd := command.New("snapshot", short, long, nil)

	return cmd
}
