package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
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
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runUsersList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunUsersList(ctx, clusterID)
}

func newUsersCreate() *cobra.Command {
	const (
		long  = `Create a new user in a Managed Postgres cluster.`
		short = "Create a user in an MPG cluster."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersCreate,
		command.RequireSession,
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
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunUsersCreate(ctx, clusterID)
}

func newUsersSetRole() *cobra.Command {
	const (
		long  = `Update a user's role in a Managed Postgres cluster.`
		short = "Update a user's role in an MPG cluster."
		usage = "set-role <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersSetRole,
		command.RequireSession,
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
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunUsersSetRole(ctx, clusterID)
}

func newUsersDelete() *cobra.Command {
	const (
		long  = `Delete a user from a Managed Postgres cluster.`
		short = "Delete a user from an MPG cluster."
		usage = "delete <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersDelete,
		command.RequireSession,
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
	if clusterID == "" {
		cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}

		clusterID = cluster.Id
	}

	return cmdv1.RunUsersDelete(ctx, clusterID)
}
