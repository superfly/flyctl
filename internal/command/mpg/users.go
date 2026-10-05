package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
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
		long  = `Create a new user in a Managed Postgres cluster.`
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
		long  = `Rotate a user's password in a Managed Postgres cluster. Prints the new password.`
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
		flag.JSONOutput(),
	)

	return cmd
}

func runUsersRotatePassword(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
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
