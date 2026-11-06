package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newUsers() *cobra.Command {
	const (
		short = "Manage users in a managed postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("users", short, long, nil)
	cmd.Aliases = []string{"user"}

	cmd.AddCommand(
		newUsersList(),
		newUsersCreate(),
		newUsersSetRole(),
		newUsersSetDatabases(),
		newUsersDelete(),
		newUsersRotatePassword(),
	)

	return cmd
}

func newUsersList() *cobra.Command {
	const (
		long  = `List users in a Managed Postgres cluster.`
		short = "List users in an MPG cluster."
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runUsersList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunUsersList(ctx, cluster.Id)
	}

	return cmdv2.RunUsersList(ctx, cluster.Id)
}

func newUsersCreate() *cobra.Command {
	const (
		long  = `Create a new user in a Managed Postgres cluster. schema_admin, writer and reader reach every database. custom users get no access until an admin user grants it with SQL; only they can be limited to some databases with --database.`
		short = "Create a user in an MPG cluster."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersCreate,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username of the user",
		},
		flag.String{
			Name:        "role",
			Shorthand:   "r",
			Description: "The role of the user (schema_admin, writer, reader, or custom)",
		},
		flag.StringArray{
			Name:        "database",
			Shorthand:   "d",
			Description: "Limit a custom user to this database; repeat for more (default: every database, including new ones)",
		},
	)

	return cmd
}

func runUsersCreate(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		if len(flag.GetStringArray(ctx, "database")) > 0 {
			return fmt.Errorf("--database is not supported for v1 clusters; migrate the cluster to v2 first")
		}

		return cmdv1.RunUsersCreate(ctx, cluster.Id)
	}

	return cmdv2.RunUsersCreate(ctx, cluster.Id)
}

func newUsersSetRole() *cobra.Command {
	const (
		long  = `Update a user's role in a Managed Postgres cluster.`
		short = "Update a user's role in an MPG cluster."
		usage = "set-role <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersSetRole,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Aliases = []string{"update-role"}
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to update",
		},
		flag.String{
			Name:        "role",
			Shorthand:   "r",
			Description: "The new role for the user (schema_admin, writer, reader, or custom)",
		},
	)

	return cmd
}

func runUsersSetRole(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunUsersSetRole(ctx, cluster.Id)
	}

	return cmdv2.RunUsersSetRole(ctx, cluster.Id)
}

func newUsersSetDatabases() *cobra.Command {
	const (
		short = "Choose which databases a custom user can connect to."
		long  = short + ` --database limits the user to the databases named; repeat it for more. --all-databases gives it every database, including ones created later. Limiting a user ends its sessions in the databases it loses.`
		usage = "set-databases <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersSetDatabases,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The custom user to update",
		},
		flag.StringArray{
			Name:        "database",
			Shorthand:   "d",
			Description: "A database the user can connect to; repeat for more",
		},
		flag.Bool{
			Name:        "all-databases",
			Description: "Let the user connect to every database, including ones created later",
		},
		flag.Yes(),
	)

	return cmd
}

func runUsersSetDatabases(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("'fly mpg users set-databases' is not supported for v1 clusters; migrate the cluster to v2 first")
	}

	return cmdv2.RunUsersSetDatabases(ctx, cluster.Id)
}

func newUsersDelete() *cobra.Command {
	const (
		long  = `Delete a user from a Managed Postgres cluster.`
		short = "Delete a user from an MPG cluster."
		usage = "delete <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersDelete,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Aliases = []string{"remove", "rm", "del"}
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to delete",
		},
		flag.Yes(),
	)

	return cmd
}

func runUsersDelete(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunUsersDelete(ctx, cluster.Id)
	}

	return cmdv2.RunUsersDelete(ctx, cluster.Id)
}

func newUsersRotatePassword() *cobra.Command {
	const (
		long = `Rotate a user's password in a Managed Postgres cluster. Prints the new password.
Update applications and DATABASE_URL secrets that use the old password.`
		short = "Rotate a user's password in an MPG cluster."
		usage = "rotate-password <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersRotatePassword,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username whose password to rotate",
		},
		flag.Bool{
			Name:        "kill-sessions",
			Description: "Terminate the user's existing database sessions after rotating the password",
		},
		flag.Yes(),
		flag.JSONOutput(),
	)

	return cmd
}

func runUsersRotatePassword(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	if config.FromContext(ctx).JSONOutput && clusterID == "" {
		return prompt.NonInteractiveError("CLUSTER_ID must be specified when using --json")
	}
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	// Unlike the sibling commands there is no v1 implementation to route to:
	// the legacy API has no password rotation endpoint.
	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("'fly mpg users rotate-password' is not supported for v1 clusters; migrate the cluster to v2 first")
	}

	return cmdv2.RunUsersRotatePassword(ctx, cluster.Id)
}
