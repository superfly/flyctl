package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func RunDetach(ctx context.Context, clusterID string, appName string) error {
	var (
		mpgClient = flapsutil.ClientFromContext(ctx)
		io        = iostreams.FromContext(ctx)
	)

	err := mpgClient.DeleteManagedPostgresAttachment(ctx, clusterID, appName)
	if err != nil {
		return fmt.Errorf("failed to detach: %w", err)
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s has been detached from %s\n", clusterID, appName)
	fmt.Fprintf(io.Out, "Note: This only removes the attachment record. Any secrets (like DATABASE_URL) are still set on the app.\n")
	fmt.Fprintf(io.Out, "Use 'fly secrets unset DATABASE_URL -a %s' to remove the connection string.\n", appName)

	return nil
}
