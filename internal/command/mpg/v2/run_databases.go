package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunDatabasesList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	databases, err := flapsClient.ListManagedPostgresDatabases(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		response, legacyErr := mpgv2.ClientFromContext(ctx).ListDatabases(ctx, clusterID)
		if legacyErr != nil {
			return fmt.Errorf("failed to list databases for cluster %s: %w", clusterID, legacyErr)
		}
		databases = make([]flaps.ManagedPostgresDatabase, 0, len(response.Data))
		for _, db := range response.Data {
			databases = append(databases, flaps.ManagedPostgresDatabase{Name: db.Name})
		}
	} else if err != nil {
		return fmt.Errorf("failed to list databases for cluster %s: %w", clusterID, err)
	}

	if len(databases) == 0 {
		fmt.Fprintf(out, "No databases found for cluster %s\n", clusterID)

		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, databases)
	}

	rows := make([][]string, 0, len(databases))
	for _, db := range databases {
		rows = append(rows, []string{
			db.Name,
		})
	}

	return render.Table(out, "", rows, "Name")
}

func RunDatabasesCreate(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

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

	created, err := flapsClient.CreateManagedPostgresDatabase(ctx, clusterID, flaps.CreateManagedPostgresDatabaseRequest{Name: dbName})
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		legacyErr := mpgv2.ClientFromContext(ctx).CreateDatabase(ctx, clusterID, mpgv2.CreateDatabaseInput{Name: dbName})
		if legacyErr != nil {
			return fmt.Errorf("failed to create database: %w", legacyErr)
		}
	} else if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	fmt.Fprintf(out, "Database created successfully!\n")
	name := created.Name
	if name == "" {
		name = dbName
	}
	fmt.Fprintf(out, "  Name: %s\n", name)

	return nil
}
