package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy a managed Postgres cluster"
		long  = short + ". " +
			`This command will permanently destroy a managed Postgres cluster and all its data.
This action is not reversible.`
		usage = "destroy <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runDestroy,
		command.RequireSession,
		command.RequireUiex,
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"delete", "remove", "rm"}

	flag.Add(cmd,
		flag.Org(),
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	var (
		clusterId  = flag.FirstArg(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		io         = iostreams.FromContext(ctx)
		colorize   = io.ColorScheme()
	)

	// Validate organization access before allowing destruction
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	// Get cluster details to verify ownership and show info
	response, err := uiexClient.GetManagedClusterById(ctx, clusterId)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	// Verify the cluster belongs to the user's org
	if response.Data.Organization.Slug != org.Slug {
		return fmt.Errorf("cluster %s does not belong to organization %s", clusterId, org.Slug)
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying a managed Postgres cluster is not reversible. All data will be permanently lost."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Destroy managed Postgres cluster %s (%s)?", response.Data.Name, clusterId); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("--yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	// Destroy the cluster
	err = uiexClient.DestroyCluster(ctx, clusterId)
	if err != nil {
		return fmt.Errorf("failed to destroy cluster %s: %w", clusterId, err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s (%s) was destroyed\n", response.Data.Name, clusterId)
	return nil
}
