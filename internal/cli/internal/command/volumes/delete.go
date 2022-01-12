package volumes

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newDelete() *cobra.Command {
	const (
		long = `Delete a volume from the application. Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Delete a volume from the app"
	)

	cmd := command.New("delete <id>", short, long, runDelete)

	return cmd
}

func runDelete(ctx context.Context) error {
	return nil
}
