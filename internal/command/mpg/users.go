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
		command.RequireUiex,
	)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runUsersList(ctx context.Context) error {
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

	users, err := uiexClient.ListUsers(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list users for cluster %s: %w", clusterID, err)
	}

	if len(users.Data) == 0 {
		fmt.Fprintf(out, "No users found for cluster %s\n", clusterID)
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, users.Data)
	}

	rows := make([][]string, 0, len(users.Data))
	for _, user := range users.Data {
		rows = append(rows, []string{
			user.Name,
			user.Role,
		})
	}

	return render.Table(out, "", rows, "Name", "Role")
}

func newUsersCreate() *cobra.Command {
	const (
		long  = `Create a new user in a Managed Postgres cluster.`
		short = "Create a user in an MPG cluster."
		usage = "create <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersCreate,
		command.RequireSession,
		command.RequireUiex,
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

	userName := flag.GetString(ctx, "username")
	if userName == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}
		err := prompt.String(ctx, &userName, "Enter username:", "", true)
		if err != nil {
			return err
		}
		if userName == "" {
			return fmt.Errorf("username cannot be empty")
		}
	}

	userRole := flag.GetString(ctx, "role")
	validRoles := map[string]bool{
		"schema_admin": true,
		"writer":       true,
		"reader":       true,
	}

	if userRole == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("user role must be specified with --role flag when not running interactively")
		}
		// Prompt for role selection
		var roleIndex int
		roleOptions := []string{"schema_admin", "writer", "reader"}
		err := prompt.Select(ctx, &roleIndex, "Select user role:", "", roleOptions...)
		if err != nil {
			return err
		}
		userRole = roleOptions[roleIndex]
	} else if !validRoles[userRole] {
		return fmt.Errorf("invalid role %q. Must be one of: schema_admin, writer, reader", userRole)
	}

	fmt.Fprintf(out, "Creating user %s with role %s in cluster %s...\n", userName, userRole, clusterID)

	input := uiex.CreateUserWithRoleInput{
		UserName: userName,
		Role:     userRole,
	}

	response, err := uiexClient.CreateUserWithRole(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Fprintf(out, "User created successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Data.Name)
	fmt.Fprintf(out, "  Role: %s\n", response.Data.Role)

	return nil
}

func newUsersSetRole() *cobra.Command {
	const (
		long  = `Update a user's role in a Managed Postgres cluster.`
		short = "Update a user's role in an MPG cluster."
		usage = "set-role <CLUSTER_ID>"
	)

	cmd := command.New(usage, short, long, runUsersSetRole,
		command.RequireSession,
		command.RequireUiex,
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

	username := flag.GetString(ctx, "username")
	if username == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}

		// Get list of users to prompt from
		usersResponse, err := uiexClient.ListUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(usersResponse.Data) == 0 {
			return fmt.Errorf("no users found in cluster %s", clusterID)
		}

		// Format users as options: "username [role]"
		var userOptions []string
		for _, user := range usersResponse.Data {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
		}

		var userIndex int
		err = prompt.Select(ctx, &userIndex, "Select user:", "", userOptions...)
		if err != nil {
			return err
		}

		username = usersResponse.Data[userIndex].Name
	}

	userRole := flag.GetString(ctx, "role")
	validRoles := map[string]bool{
		"schema_admin": true,
		"writer":       true,
		"reader":       true,
	}

	if userRole == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("user role must be specified with --role flag when not running interactively")
		}
		// Prompt for role selection
		var roleIndex int
		roleOptions := []string{"schema_admin", "writer", "reader"}
		err := prompt.Select(ctx, &roleIndex, "Select user role:", "", roleOptions...)
		if err != nil {
			return err
		}
		userRole = roleOptions[roleIndex]
	} else if !validRoles[userRole] {
		return fmt.Errorf("invalid role %q. Must be one of: schema_admin, writer, reader", userRole)
	}

	fmt.Fprintf(out, "Updating user %s role to %s in cluster %s...\n", username, userRole, clusterID)

	input := uiex.UpdateUserRoleInput{
		Role: userRole,
	}

	response, err := uiexClient.UpdateUserRole(ctx, clusterID, username, input)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	fmt.Fprintf(out, "User role updated successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Data.Name)
	fmt.Fprintf(out, "  Role: %s\n", response.Data.Role)

	return nil
}
