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
		short = "Manage users in a Managed Postgres cluster"
		long  = `List, create and delete database users in a Managed Postgres cluster,
change their roles, and rotate their passwords. Each user has one role:
schema_admin, writer or reader.`
	)

	cmd := command.New("users", short, long, nil)
	cmd.Aliases = []string{"user"}

	cmd.AddCommand(
		newUsersList(),
		newUsersCreate(),
		newUsersSetRole(),
		newUsersDelete(),
		newUsersRotatePassword(),
	)

	return cmd
}

func newUsersList() *cobra.Command {
	const (
		long = `List the database users in a Managed Postgres cluster with each user's
role.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.`
		short = "List users in a Managed Postgres cluster"
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
		long = `Create a database user in a Managed Postgres cluster with a role of
schema_admin, writer or reader. Pass --username and --role, or enter them
when asked in a terminal; without a terminal, both are required.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.

To give an app a connection string for the new user, use
` + "`fly mpg attach --username`" + `.`
		short = "Create a user in a Managed Postgres cluster"
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
			Description: "The role of the user (schema_admin, writer, or reader)",
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
		return cmdv1.RunUsersCreate(ctx, cluster.Id)

	}

	return cmdv2.RunUsersCreate(ctx, cluster.Id)
}

func newUsersSetRole() *cobra.Command {
	const (
		long = `Change a database user's role in a Managed Postgres cluster to
schema_admin, writer or reader. Pass --username and --role, or pick them
from lists in a terminal; without a terminal, both are required.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.`
		short = "Change a user's role in a Managed Postgres cluster"
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
			Description: "The new role for the user (schema_admin, writer, or reader)",
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

func newUsersDelete() *cobra.Command {
	const (
		long = `Delete a database user from a Managed Postgres cluster. This can't be
undone. Pass --username, or pick the user from a list in a terminal.
You're asked to confirm unless you pass --yes; without a terminal,
--username and --yes are required.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required.`
		short = "Delete a user from a Managed Postgres cluster"
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
		long = `Give a Managed Postgres user a new password and print it. This can't be
undone: apps using the old password can't open new connections, so update
their DATABASE_URL secrets afterwards. Existing sessions stay open unless
you pass --kill-sessions.

Pass --username, or pick the user from a list in a terminal. You're asked
to confirm unless you pass --yes. Without a terminal, --username and --yes
are required; with --json, the cluster ID is required too.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required. Not available for v1 clusters.`
		short = "Rotate a user's password in a Managed Postgres cluster"
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
