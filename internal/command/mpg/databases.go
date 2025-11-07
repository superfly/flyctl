package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newDatabases() *cobra.Command {
	const (
		short = "Manage databases in a managed postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("databases", short, long, nil)
	cmd.Aliases = []string{"database", "db", "dbs"}

	cmd.AddCommand(
		newDatabasesList(),
		newDatabasesCreate(),
	)

	return cmd
}

func newDatabasesList() *cobra.Command {
	const (
		long  = `List databases in a Managed Postgres cluster.`
		short = "List databases in an MPG cluster."
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runDatabasesList,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runDatabasesList(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	uiexClient := uiexutil.ClientFromContext(ctx)

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	databases, err := uiexClient.ListDatabases(ctx, clusterID)
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

func newDatabasesCreate() *cobra.Command {
	const (
		long  = `Create a new database in a Managed Postgres cluster.`
		short = "Create a database in an MPG cluster."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runDatabasesCreate,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the database",
		},
	)

	return cmd
}

func runDatabasesCreate(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	uiexClient := uiexutil.ClientFromContext(ctx)

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	dbName := flag.GetString(ctx, "name")
	if dbName == "" {
		err := prompt.String(ctx, &dbName, "Enter database name:", "", true)
		if err != nil {
			return err
		}
		if dbName == "" {
			return fmt.Errorf("database name cannot be empty")
		}
	}

	fmt.Fprintf(out, "Creating database %s in cluster %s...\n", dbName, clusterID)

	input := uiex.CreateDatabaseInput{
		Name: dbName,
	}

	response, err := uiexClient.CreateDatabase(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	fmt.Fprintf(out, "Database created successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Data.Name)

	return nil
}
