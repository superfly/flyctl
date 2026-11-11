package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newDatabases() *cobra.Command {
	const (
		short = "Manage databases in a Managed Postgres cluster"
		long  = `List and create databases in a Managed Postgres cluster, and manage
their Postgres extensions with ` + "`fly mpg databases extensions`" + `.`
	)

	cmd := command.New("databases", short, long, nil)
	cmd.Aliases = []string{"database", "db", "dbs"}

	cmd.AddCommand(
		newDatabasesList(),
		newDatabasesCreate(),
		newExtensions(),
	)

	return cmd
}

func newDatabasesList() *cobra.Command {
	const (
		long = `List the databases in a Managed Postgres cluster by name.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.`
		short = "List databases in a Managed Postgres cluster"
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runDatabasesList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runDatabasesList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunDatabasesList(ctx, cluster.Id)
	}

	return cmdv2.RunDatabasesList(ctx, cluster.Id)
}

func newDatabasesCreate() *cobra.Command {
	const (
		long = `Create a database in a Managed Postgres cluster. Pass the name with
--name, or enter it when asked in a terminal; without a terminal, --name
is required.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.

To give an app a connection string for the new database, use
` + "`fly mpg attach --database`" + `.`
		short = "Create a database in a Managed Postgres cluster"
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runDatabasesCreate,
		command.RequireSession,
		requireMacaroonToken,
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
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunDatabasesCreate(ctx, cluster.Id)
	}

	return cmdv2.RunDatabasesCreate(ctx, cluster.Id)
}
