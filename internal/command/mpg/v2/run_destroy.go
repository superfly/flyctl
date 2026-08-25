package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunDestroy(ctx context.Context, clusterId string) error {
	var (
		mpgClient    = flapsutil.ClientFromContext(ctx)
		legacyClient = mpgv2.ClientFromContext(ctx)
		io           = iostreams.FromContext(ctx)
		colorize     = io.ColorScheme()
	)

	// Get cluster details to verify ownership and show info
	cluster, err := mpgClient.GetManagedPostgresCluster(ctx, clusterId)
	useLegacy := errors.Is(err, flaps.ErrFlapsNotFound)
	if err != nil && !useLegacy {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}
	if useLegacy {
		response, err := legacyClient.GetClusterById(ctx, clusterId)
		if err != nil {
			return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
		}
		cluster.Name = response.Data.Name
		cluster.Organization.Name = response.Data.Organization.Name
		cluster.Organization.Slug = response.Data.Organization.Slug
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying a managed Postgres cluster is not reversible. All data will be permanently lost."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Destroy managed Postgres cluster %s from organization %s (%s)?", cluster.Name, cluster.Organization.Name, clusterId); {
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
	if useLegacy {
		err = legacyClient.DestroyCluster(ctx, cluster.Organization.Slug, clusterId)
	} else {
		err = mpgClient.DeleteManagedPostgresCluster(ctx, clusterId)
	}
	if err != nil {
		return fmt.Errorf("failed to destroy cluster %s: %w", clusterId, err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s (%s) scheduled to be destroyed (may take some time)\n", cluster.Name, clusterId)

	return nil
}
