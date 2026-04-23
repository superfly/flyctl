package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
)

func RunDatabasesList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	databases, err := mpgClient.ListDatabases(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list databases for cluster %s: %w", clusterID, err)
	}

	if len(databases.Data) == 0 {
		fmt.Fprintf(out, "No databases found for cluster %s\n", clusterID)

		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, databases.Data)
	}

	rows := make([][]string, 0, len(databases.Data))
	for _, db := range databases.Data {
		rows = append(rows, []string{
			db.Name,
		})
	}

	return render.Table(out, "", rows, "Name")
}

func RunDatabasesCreate(ctx context.Context, clusterID string) error {
	return nil
}
