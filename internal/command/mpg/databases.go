package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/mpg/utils"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
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
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func newDatabasesCreate() *cobra.Command {
	const (
		long  = `Create a new database in a Managed Postgres cluster.`
		short = "Create a database in an MPG cluster."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runDatabasesCreate,
		command.RequireSession,
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

func runDatabasesList(ctx context.Context) error {
	var cluster *utils.ManagedCluster
	var err error

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err = utils.ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}
	if cluster.Version == utils.V1 {
		return cmdv1.RunDatabasesList(ctx, clusterID)
	}

	return cmdv2.RunDatabasesList(ctx, clusterID)
}

func runDatabasesCreate(ctx context.Context) error {
	var cluster *utils.ManagedCluster
	var err error

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err = utils.ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}
	if cluster.Version == utils.V1 {
		return cmdv1.RunDatabasesCreate(ctx, clusterID)
	}

	return cmdv2.RunDatabasesCreate(ctx, clusterID)

}
