package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// Copied from v1 RunDatabasesList and changed client to mpgv2
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

// Copied from v1 RunDatabasesCreate and changed client to mpgv2
func RunDatabasesCreate(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	dbName := flag.GetString(ctx, "name")
	if dbName == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("database name must be specified with --name flag when not running interactively")
		}
		err := prompt.String(ctx, &dbName, "Enter database name:", "", true)
		if err != nil {
			return err
		}
		if dbName == "" {
			return fmt.Errorf("database name cannot be empty")
		}
	}

	fmt.Fprintf(out, "Creating database %s in cluster %s...\n", dbName, clusterID)

	input := mpgv2.CreateDatabaseInput{
		Name: dbName,
	}

	response, err := mpgClient.CreateDatabase(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	fmt.Fprintf(out, "Database created successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Data.Name)

	return nil
}
