package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func RunExtensionsList(ctx context.Context, clusterID, database string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv1.ClientFromContext(ctx)

	database, err := resolveDatabase(ctx, clusterID, database)
	if err != nil {
		return err
	}

	resp, err := mpgClient.ListExtensions(ctx, clusterID, database)
	if err != nil {
		return fmt.Errorf("failed to list extensions for database %s: %w", database, err)
	}

	if cfg.JSONOutput {
		return render.JSON(out, resp.Data)
	}

	rows := make([][]string, 0, len(resp.Data))
	for _, ext := range resp.Data {
		installed := "no"
		version := ""
		schema := ""
		if ext.Installed != nil {
			installed = "yes"
			version = ext.Installed.Version
			schema = ext.Installed.Schema
		}
		rows = append(rows, []string{
			ext.Name,
			installed,
			version,
			schema,
			ext.Description,
		})
	}

	return render.Table(out, fmt.Sprintf("Extensions in database %s", database), rows,
		"Name", "Installed", "Version", "Schema", "Description")
}

func RunExtensionsEnable(ctx context.Context, clusterID, database, name, schema string, createSchema bool) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv1.ClientFromContext(ctx)

	database, err := resolveDatabase(ctx, clusterID, database)
	if err != nil {
		return err
	}

	// postgis_topology must live in the "topology" schema. Default to that when
	// the user didn't explicitly pick a schema so the command works out of the box.
	if name == "postgis_topology" && schema == "" {
		schema = "topology"
		createSchema = true
	}

	input := mpgv1.EnableExtensionInput{
		Name:                 name,
		Schema:               schema,
		CreateSchemaIfNeeded: createSchema,
	}

	if err := mpgClient.EnableExtension(ctx, clusterID, database, input); err != nil {
		return err
	}

	fmt.Fprintf(out, "Extension %s enabled on database %s.\n", name, database)
	
	return nil
}

func RunExtensionsDisable(ctx context.Context, clusterID, database, name string, force bool) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv1.ClientFromContext(ctx)

	database, err := resolveDatabase(ctx, clusterID, database)
	if err != nil {
		return err
	}

	if err := mpgClient.DisableExtension(ctx, clusterID, database, name, force); err != nil {
		return err
	}

	fmt.Fprintf(out, "Extension %s disabled on database %s.\n", name, database)
	
	return nil
}

// resolveDatabase returns the database to target: the explicit flag value if
// given, the cluster's only database if there's exactly one, or an interactive
// pick from the list. Returns an error if running non-interactively without a
// flag and the cluster has multiple databases.
func resolveDatabase(ctx context.Context, clusterID, database string) (string, error) {
	if database != "" {
		return database, nil
	}

	mpgClient := mpgv1.ClientFromContext(ctx)
	dbs, err := mpgClient.ListDatabases(ctx, clusterID)
	if err != nil {
		return "", fmt.Errorf("failed to list databases: %w", err)
	}

	if len(dbs.Data) == 0 {
		return "", fmt.Errorf("no databases found in cluster %s", clusterID)
	}

	if len(dbs.Data) == 1 {
		return dbs.Data[0].Name, nil
	}

	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		return "", prompt.NonInteractiveError("the cluster has multiple databases; pass --database to choose one")
	}

	options := make([]string, 0, len(dbs.Data))
	for _, db := range dbs.Data {
		options = append(options, db.Name)
	}

	var idx int
	if err := prompt.Select(ctx, &idx, "Select database:", "", options...); err != nil {
		return "", err
	}
	
	return dbs.Data[idx].Name, nil
}
