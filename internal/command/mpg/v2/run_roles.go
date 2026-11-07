package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// RunRolesList lists the roles made with SQL in a cluster. It calls the
// Machines API only.
func RunRolesList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	roles, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresRoles(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list roles for cluster %s: %w", clusterID, withClusterUpdateHint(err))
	}

	if cfg.JSONOutput {
		return render.JSON(out, roles)
	}

	if len(roles) == 0 {
		fmt.Fprintf(out, "No roles found in cluster %s\n", clusterID)

		return nil
	}

	rows := make([][]string, 0, len(roles))
	for _, role := range roles {
		rows = append(rows, []string{role.Name, fmt.Sprint(role.MemberCount)})
	}

	return render.Table(out, "", rows, "Name", "Members")
}

// RunRolesMembers lists the direct members of a role made with SQL. It calls
// the Machines API only.
func RunRolesMembers(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	io := iostreams.FromContext(ctx)
	out := io.Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	role := flag.GetString(ctx, "role")
	if role == "" {
		if cfg.JSONOutput {
			return prompt.NonInteractiveError("role must be specified with --role flag when using --json")
		}
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("role must be specified with --role flag when not running interactively")
		}

		roles, err := flapsClient.ListManagedPostgresRoles(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list roles for cluster %s: %w", clusterID, withClusterUpdateHint(err))
		}
		if len(roles) == 0 {
			return fmt.Errorf("no roles found in cluster %s", clusterID)
		}

		options := make([]string, 0, len(roles))
		for _, r := range roles {
			members := "members"
			if r.MemberCount == 1 {
				members = "member"
			}
			options = append(options, fmt.Sprintf("%s (%d %s)", r.Name, r.MemberCount, members))
		}

		var index int
		if err := prompt.Select(ctx, &index, "Select role:", "", options...); err != nil {
			return err
		}
		role = roles[index].Name
	}

	members, err := flapsClient.ListManagedPostgresRoleMembers(ctx, clusterID, role)
	if err != nil {
		return fmt.Errorf("failed to list members of role %s in cluster %s: %w", role, clusterID, withClusterUpdateHint(err))
	}

	if cfg.JSONOutput {
		return render.JSON(out, members)
	}

	if len(members) == 0 {
		fmt.Fprintf(out, "Role %s has no members\n", role)

		return nil
	}

	rows := make([][]string, 0, len(members))
	for _, member := range members {
		rows = append(rows, []string{member.Name})
	}

	return render.Table(out, "", rows, "Name")
}
