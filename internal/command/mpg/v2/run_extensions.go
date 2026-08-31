package cmdv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunExtensionsList(ctx context.Context, clusterID, database string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	database, err := resolveDatabase(ctx, clusterID, database)
	if err != nil {
		return err
	}

	extensions, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresExtensions(ctx, clusterID, database)
	var outputExtensions []mpgv2.Extension
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		resp, legacyErr := mpgv2.ClientFromContext(ctx).ListExtensions(ctx, clusterID, database)
		if legacyErr != nil {
			return fmt.Errorf("failed to list extensions for database %s: %w", database, legacyErr)
		}
		outputExtensions = resp.Data
	} else if err != nil {
		return fmt.Errorf("failed to list extensions for database %s: %w", database, err)
	} else {
		outputExtensions = make([]mpgv2.Extension, 0, len(extensions))
		for _, ext := range extensions {
			outputExtensions = append(outputExtensions, extensionForOutput(ext))
		}
	}

	if cfg.JSONOutput {
		return render.JSON(out, outputExtensions)
	}

	rows := make([][]string, 0, len(outputExtensions))
	for _, ext := range outputExtensions {
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

	input := flaps.EnableManagedPostgresExtensionRequest{
		Name:         name,
		Schema:       schema,
		CreateSchema: createSchema,
	}

	err = flapsutil.ClientFromContext(ctx).EnableManagedPostgresExtension(ctx, clusterID, database, input)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		err = mpgv2.ClientFromContext(ctx).EnableExtension(ctx, clusterID, database, mpgv2.EnableExtensionInput{
			Name:                 name,
			Schema:               schema,
			CreateSchemaIfNeeded: createSchema,
		})
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Extension %s enabled on database %s.\n", name, database)

	return nil
}

func RunExtensionsDisable(ctx context.Context, clusterID, database, name string, force bool) error {
	out := iostreams.FromContext(ctx).Out

	database, err := resolveDatabase(ctx, clusterID, database)
	if err != nil {
		return err
	}

	err = flapsutil.ClientFromContext(ctx).DisableManagedPostgresExtension(ctx, clusterID, database, name, force)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		err = mpgv2.ClientFromContext(ctx).DisableExtension(ctx, clusterID, database, name, force)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Extension %s disabled on database %s.\n", name, database)

	return nil
}

func extensionForOutput(ext flaps.ManagedPostgresExtension) mpgv2.Extension {
	legacy := mpgv2.Extension{
		Name:           ext.Name,
		Description:    optionalString(ext.Description),
		DefaultVersion: optionalString(ext.DefaultVersion),
		IsSystem:       ext.System,
	}
	if ext.Installed != nil {
		legacy.Installed = &mpgv2.InstalledExtension{
			Version: ext.Installed.Version,
			Schema:  ext.Installed.Schema,
		}
	}

	return legacy
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// resolveDatabase returns the database to target: the explicit flag value if
// given, the cluster's only database if there's exactly one, or an interactive
// pick from the list. Returns an error if running non-interactively without a
// flag and the cluster has multiple databases.
func resolveDatabase(ctx context.Context, clusterID, database string) (string, error) {
	if database != "" {
		return database, nil
	}

	databases, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresDatabases(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		response, legacyErr := mpgv2.ClientFromContext(ctx).ListDatabases(ctx, clusterID)
		if legacyErr != nil {
			return "", fmt.Errorf("failed to list databases: %w", legacyErr)
		}
		databases = make([]flaps.ManagedPostgresDatabase, 0, len(response.Data))
		for _, database := range response.Data {
			databases = append(databases, flaps.ManagedPostgresDatabase{Name: database.Name})
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to list databases: %w", err)
	}

	if len(databases) == 0 {
		return "", fmt.Errorf("no databases found in cluster %s", clusterID)
	}

	if len(databases) == 1 {
		return databases[0].Name, nil
	}

	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		return "", prompt.NonInteractiveError("the cluster has multiple databases; pass --database to choose one")
	}

	options := make([]string, 0, len(databases))
	for _, db := range databases {
		options = append(options, db.Name)
	}

	var idx int
	if err := prompt.Select(ctx, &idx, "Select database:", "", options...); err != nil {
		return "", err
	}

	return databases[idx].Name, nil
}
