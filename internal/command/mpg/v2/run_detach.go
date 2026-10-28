package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunDetach(ctx context.Context, clusterID string, appName string) error {
	var (
		mpgClient    = flapsutil.ClientFromContext(ctx)
		legacyClient = mpgv2.ClientFromContext(ctx)
		io           = iostreams.FromContext(ctx)
	)

	// Delete the attachment record. Prefer the public Machines API;
	// fall back to the legacy MPGv2 client only when it returns a
	// classified 404, signaling the endpoint isn't available on this
	// API surface. Any other error is authoritative and must propagate.
	err := mpgClient.DeleteManagedPostgresAttachment(ctx, clusterID, appName)
	useLegacy := errors.Is(err, flaps.ErrFlapsNotFound)
	if err != nil && !useLegacy {
		return fmt.Errorf("failed to detach: %w", err)
	}
	if useLegacy {
		if _, err := legacyClient.DeleteAttachment(ctx, clusterID, appName); err != nil {
			return fmt.Errorf("failed to detach: %w", err)
		}
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s has been detached from %s\n", clusterID, appName)
	fmt.Fprintf(io.Out, "Note: This only removes the attachment record. DATABASE_URL, and DIRECT_URL if attach set it, remain on the app (or your custom secret names).\n")
	fmt.Fprintf(io.Out, "To remove them, run 'fly secrets unset DATABASE_URL -a %s', and DIRECT_URL too if attach set it (use your custom names if needed).\n", appName)

	return nil
}
