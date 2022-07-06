package builder

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newUpdate() *cobra.Command {
	const (
		long  = "Update an organization's remote builder image"
		short = long

		usage = "update <org-name> <image_ref>"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
	)

	cmd.Args = cobra.MinimumNArgs(2)

	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	orgName := flag.FirstArg(ctx)
	image := flag.Args(ctx)[1]

	org, err := client.UpdateRemoteBuilder(ctx, orgName, image)

	if err != nil {
		return fmt.Errorf("failed updating remote builder: %w", err)
	}

	fmt.Fprintf(io.Out, "\nUpdated remote builder image to: %s\n", org.RemoteBuilderImage)
	fmt.Fprintln(io.Out, "For this change to take effect, you'll need to destroy the current remote builder app with 'fly apps destroy'.")

	return nil
}
