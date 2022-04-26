package builder

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
)

func newShow() *cobra.Command {
	const (
		long  = "Show details about an organization's remote builder image"
		short = long

		usage = "show <org-name>"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runShow(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	orgName := flag.FirstArg(ctx)

	org, err := client.GetOrganizationBySlug(ctx, orgName)

	if err != nil {
		return fmt.Errorf("failed getting org: %w", err)
	}

	if org.RemoteBuilderApp != nil {
		fmt.Fprint(io.Out, "App name: ", org.RemoteBuilderApp.Name)
	} else {
		fmt.Fprintln(io.Out, "This org has not deployed apps yet.")
	}
	fmt.Fprint(io.Out, "Desired remote builder image: ", org.RemoteBuilderImage)

	return nil
}
