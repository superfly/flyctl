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

func newExtensions() *cobra.Command {
	const (
		short = "Manage Postgres extensions in a Managed Postgres database"
		long  = `List, enable and disable Postgres extensions in a database of a Managed
Postgres cluster. Start with ` + "`fly mpg databases extensions list`" + ` to see which
extensions are available and installed.`
	)

	cmd := command.New("extensions", short, long, nil)
	cmd.Aliases = []string{"extension", "ext"}

	cmd.AddCommand(
		newExtensionsList(),
		newExtensionsEnable(),
		newExtensionsDisable(),
	)

	return cmd
}

func newExtensionsList() *cobra.Command {
	const (
		long = `List the Postgres extensions available in a Managed Postgres database,
showing which are installed and, for those, their version and schema.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.
With no --database, the cluster's only database is used; if there are
several, you pick one in a terminal, and without a terminal --database is
required.`
		short = "List Postgres extensions in a Managed Postgres database"
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runExtensionsList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "Target database within the cluster",
		},
		flag.JSONOutput(),
	)

	return cmd
}

func runExtensionsList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	database := flag.GetString(ctx, "database")

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunExtensionsList(ctx, cluster.Id, database)
	}

	return cmdv2.RunExtensionsList(ctx, cluster.Id, database)
}

func newExtensionsEnable() *cobra.Command {
	const (
		long = `Enable (install) a Postgres extension in a Managed Postgres database. The
extension goes in the schema given by --schema; add --create-schema if
that schema doesn't exist yet. For postgis_topology with no --schema, the
topology schema is used and created if needed.

With no --cluster, you pick an organization and a cluster from a list in
a terminal; without a terminal, --cluster is required.
With no --database, the cluster's only database is used; if there are
several, you pick one in a terminal, and without a terminal --database is
required.

See available extensions with ` + "`fly mpg databases extensions list`" + `.`
		short = "Enable a Postgres extension in a Managed Postgres database"
		usage = "enable <EXTENSION>"
	)

	cmd := command.New(usage, short, long, runExtensionsEnable,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "cluster",
			Shorthand:   "c",
			Description: "Target cluster ID",
		},
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "Target database within the cluster",
		},
		flag.String{
			Name:        "schema",
			Description: "Schema in which to create the extension (default: public)",
		},
		flag.Bool{
			Name:        "create-schema",
			Description: "Create the schema if it does not exist",
			Default:     false,
		},
	)

	return cmd
}

func runExtensionsEnable(ctx context.Context) error {
	extensionName := flag.FirstArg(ctx)
	clusterID := flag.GetString(ctx, "cluster")
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	database := flag.GetString(ctx, "database")
	schema := flag.GetString(ctx, "schema")
	createSchema := flag.GetBool(ctx, "create-schema")

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunExtensionsEnable(ctx, cluster.Id, database, extensionName, schema, createSchema)
	}

	return cmdv2.RunExtensionsEnable(ctx, cluster.Id, database, extensionName, schema, createSchema)
}

func newExtensionsDisable() *cobra.Command {
	const (
		long = `Disable (drop) a Postgres extension in a Managed Postgres database. With
--force, objects that depend on the extension are dropped too (CASCADE).
There's no confirmation prompt.

With no --cluster, you pick an organization and a cluster from a list in
a terminal; without a terminal, --cluster is required.
With no --database, the cluster's only database is used; if there are
several, you pick one in a terminal, and without a terminal --database is
required.`
		short = "Disable a Postgres extension in a Managed Postgres database"
		usage = "disable <EXTENSION>"
	)

	cmd := command.New(usage, short, long, runExtensionsDisable,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "cluster",
			Shorthand:   "c",
			Description: "Target cluster ID",
		},
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "Target database within the cluster",
		},
		flag.Bool{
			Name:        "force",
			Description: "Drop dependent objects as well (CASCADE)",
			Default:     false,
		},
	)

	return cmd
}

func runExtensionsDisable(ctx context.Context) error {
	extensionName := flag.FirstArg(ctx)
	clusterID := flag.GetString(ctx, "cluster")
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	database := flag.GetString(ctx, "database")
	force := flag.GetBool(ctx, "force")

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunExtensionsDisable(ctx, cluster.Id, database, extensionName, force)
	}

	return cmdv2.RunExtensionsDisable(ctx, cluster.Id, database, extensionName, force)
}
