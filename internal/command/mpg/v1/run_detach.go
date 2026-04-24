package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func RunDetach(ctx context.Context, clusterID string, appName string) error {
	io := iostreams.FromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Delete the attachment record
	_, err := uiexClient.DeleteAttachment(ctx, clusterID, appName)
	if err != nil {
		return fmt.Errorf("failed to detach: %w", err)
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s has been detached from %s\n", clusterID, appName)
	fmt.Fprintf(io.Out, "Note: This only removes the attachment record. Any secrets (like DATABASE_URL) are still set on the app.\n")
	fmt.Fprintf(io.Out, "Use 'fly secrets unset DATABASE_URL -a %s' to remove the connection string.\n", appName)

	return nil
}
