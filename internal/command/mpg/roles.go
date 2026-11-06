package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newRoles() *cobra.Command {
	const (
		short = "Show roles made with SQL in a managed postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("roles", short, long, nil)
	cmd.Aliases = []string{"role"}

	cmd.AddCommand(
		newRolesList(),
		newRolesMembers(),
	)

	return cmd
}

func newRolesList() *cobra.Command {
	const (
		long  = `List the roles made with SQL in a Managed Postgres cluster and how many members each has. Built-in and system roles aren't listed.`
		short = "List roles made with SQL in an MPG cluster."
		usage = "list <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runRolesList,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runRolesList(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("'fly mpg roles list' is not supported for v1 clusters; migrate the cluster to v2 first")
	}

	return cmdv2.RunRolesList(ctx, cluster.Id)
}

func newRolesMembers() *cobra.Command {
	const (
		long  = `List the direct members of a role made with SQL: users, other roles, and built-in roles such as writer.`
		short = "List the members of a role in an MPG cluster."
		usage = "members <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runRolesMembers,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "role",
			Shorthand:   "r",
			Description: "The role made with SQL, named exactly as in Postgres",
		},
		flag.JSONOutput(),
	)

	return cmd
}

func runRolesMembers(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("'fly mpg roles members' is not supported for v1 clusters; migrate the cluster to v2 first")
	}

	return cmdv2.RunRolesMembers(ctx, cluster.Id)
}
