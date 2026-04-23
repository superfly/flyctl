package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func RunDestroy(ctx context.Context, clusterID string) error {
	var (
		mpgClient = mpgv1.ClientFromContext(ctx)
		io        = iostreams.FromContext(ctx)
		colorize  = io.ColorScheme()
	)

	// Get cluster details to verify ownership and show info
	response, err := mpgClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying a managed Postgres cluster is not reversible. All data will be permanently lost."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Destroy managed Postgres cluster %s from organization %s (%s)?", response.Data.Name, response.Data.Organization.Name, clusterID); {
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
	err = mpgClient.DestroyCluster(ctx, response.Data.Organization.Slug, clusterID)
	if err != nil {
		return fmt.Errorf("failed to destroy cluster %s: %w", clusterID, err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s (%s) scheduled to be destroyed (may take some time)\n", response.Data.Name, clusterID)

	return nil
}
