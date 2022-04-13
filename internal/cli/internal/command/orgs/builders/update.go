package builders

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
)

func newUpdate() *cobra.Command {
	const (
		long  = "Update an organization's remote builder image"
		short = long

		usage = "update <org-name>"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	orgName := flag.FirstArg(ctx)

	org, err := client.UpdateRemoteBuilder(ctx, orgName)

	if err != nil {
		return fmt.Errorf("failed updating remote builder: %w", err)
	}

	fmt.Fprintf(io.Out, "Updated image to: ", org.Settings)

	return nil
}
