package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
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
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	var (
		clusterId  = flag.FirstArg(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		io         = iostreams.FromContext(ctx)
		colorize   = io.ColorScheme()
	)

	// Get cluster details to verify ownership and show info
	response, err := uiexClient.GetManagedClusterById(ctx, clusterId)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying a managed Postgres cluster is not reversible. All data will be permanently lost."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Destroy managed Postgres cluster %s from organization %s (%s)?", response.Data.Name, response.Data.Organization.Name, clusterId); {
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
	err = uiexClient.DestroyCluster(ctx, response.Data.Organization.Slug, clusterId)
	if err != nil {
		return fmt.Errorf("failed to destroy cluster %s: %w", clusterId, err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s (%s) scheduled to be destroyed (may take some time)\n", response.Data.Name, clusterId)
	return nil
}
