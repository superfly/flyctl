package cmdv2

import (
	"context"
	"fmt"
	stdio "io"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// managedPostgresUserRoleOptions lists the roles the CLI exposes to the user
// when prompting interactively, in the order they are presented. Keeping this
// derived from the flaps package constants ensures the prompt and the request
// body cannot drift from the API's accepted set.
var managedPostgresUserRoleOptions = []string{
	string(flaps.ManagedPostgresUserRoleSchemaAdmin),
	string(flaps.ManagedPostgresUserRoleWriter),
	string(flaps.ManagedPostgresUserRoleReader),
}

// publicUserToLegacy converts the public response to the legacy output shape.
func publicUserToLegacy(u flaps.ManagedPostgresUser) mpgv2.User {
	return mpgv2.User{Name: u.Username, Role: string(u.Role)}
}

func listUsers(ctx context.Context, clusterID string) ([]mpgv2.User, error) {
	publicUsers, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresUsers(ctx, clusterID)
	if flapsutil.IsFlapsRouteMissing(err) {
		response, legacyErr := mpgv2.ClientFromContext(ctx).ListUsers(ctx, clusterID)
		if legacyErr != nil {
			return nil, legacyErr
		}

		return response.Data, nil
	}
	if err != nil {
		return nil, err
	}

	users := make([]mpgv2.User, 0, len(publicUsers))
	for _, user := range publicUsers {
		users = append(users, publicUserToLegacy(user))
	}

	return users, nil
}

func RunUsersList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	users, err := listUsers(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list users for cluster %s: %w", clusterID, err)
	}

	if len(users) == 0 {
		fmt.Fprintf(out, "No users found for cluster %s\n", clusterID)

		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, users)
	}

	rows := make([][]string, 0, len(users))
	for _, user := range users {
		rows = append(rows, []string{
			user.Name,
			user.Role,
		})
	}

	return render.Table(out, "", rows, "Name", "Role")
}

func RunUsersCreate(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	username := flag.GetString(ctx, "username")
	if username == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}
		err := prompt.String(ctx, &username, "Enter username:", "", true)
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}
	}

	role := flag.GetString(ctx, "role")
	validRoles := map[string]bool{
		string(flaps.ManagedPostgresUserRoleSchemaAdmin): true,
		string(flaps.ManagedPostgresUserRoleWriter):      true,
		string(flaps.ManagedPostgresUserRoleReader):      true,
	}

	if role == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("user role must be specified with --role flag when not running interactively")
		}
		// Prompt for role selection
		var roleIndex int
		err := prompt.Select(ctx, &roleIndex, "Select user role:", "", managedPostgresUserRoleOptions...)
		if err != nil {
			return err
		}
		role = managedPostgresUserRoleOptions[roleIndex]
	} else if !validRoles[role] {
		return fmt.Errorf("invalid role %q. Must be one of: %s, %s, %s", role, flaps.ManagedPostgresUserRoleSchemaAdmin, flaps.ManagedPostgresUserRoleWriter, flaps.ManagedPostgresUserRoleReader)
	}

	fmt.Fprintf(out, "Creating user %s with role %s in cluster %s...\n", username, role, clusterID)

	input := flaps.CreateManagedPostgresUserRequest{
		Username: username,
		Role:     role,
	}

	response, err := flapsClient.CreateManagedPostgresUser(ctx, clusterID, input)
	if flapsutil.IsFlapsRouteMissing(err) {
		legacyResponse, legacyErr := mpgv2.ClientFromContext(ctx).CreateUserWithRole(ctx, clusterID, mpgv2.CreateUserWithRoleInput(input))
		if legacyErr != nil {
			return fmt.Errorf("failed to create user: %w", legacyErr)
		}
		response = flaps.ManagedPostgresUser{
			Username: legacyResponse.Data.Name,
			Role:     flaps.ManagedPostgresUserRole(legacyResponse.Data.Role),
		}
	} else if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Fprintf(out, "User created successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Username)
	fmt.Fprintf(out, "  Role: %s\n", string(response.Role))

	return nil
}

func RunUsersSetRole(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	username := flag.GetString(ctx, "username")
	if username == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}

		// Get list of users to prompt from
		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(users) == 0 {
			return fmt.Errorf("no users found in cluster %s", clusterID)
		}

		// Format users as options: "username [role]"
		var userOptions []string
		for _, user := range users {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
		}

		var userIndex int
		err = prompt.Select(ctx, &userIndex, "Select user:", "", userOptions...)
		if err != nil {
			return err
		}

		username = users[userIndex].Name
	}

	role := flag.GetString(ctx, "role")
	validRoles := map[string]bool{
		string(flaps.ManagedPostgresUserRoleSchemaAdmin): true,
		string(flaps.ManagedPostgresUserRoleWriter):      true,
		string(flaps.ManagedPostgresUserRoleReader):      true,
	}

	if role == "" {
		io := iostreams.FromContext(ctx)
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("user role must be specified with --role flag when not running interactively")
		}
		// Prompt for role selection
		var roleIndex int
		err := prompt.Select(ctx, &roleIndex, "Select user role:", "", managedPostgresUserRoleOptions...)
		if err != nil {
			return err
		}
		role = managedPostgresUserRoleOptions[roleIndex]
	} else if !validRoles[role] {
		return fmt.Errorf("invalid role %q. Must be one of: %s, %s, %s", role, flaps.ManagedPostgresUserRoleSchemaAdmin, flaps.ManagedPostgresUserRoleWriter, flaps.ManagedPostgresUserRoleReader)
	}

	fmt.Fprintf(out, "Updating user %s role to %s in cluster %s...\n", username, role, clusterID)

	input := flaps.UpdateManagedPostgresUserRoleRequest{
		Role: role,
	}

	err := flapsClient.UpdateManagedPostgresUserRole(ctx, clusterID, username, input)
	if flapsutil.IsFlapsRouteMissing(err) {
		err = mpgv2.ClientFromContext(ctx).UpdateUserRole(ctx, clusterID, username, mpgv2.UpdateUserRoleInput(input))
	}
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	fmt.Fprintf(out, "User role updated successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", username)
	fmt.Fprintf(out, "  Role: %s\n", role)

	return nil
}

func RunUsersDelete(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	flapsClient := flapsutil.ClientFromContext(ctx)

	username := flag.GetString(ctx, "username")
	if username == "" {
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}

		// Get list of users to prompt from
		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(users) == 0 {
			return fmt.Errorf("no users found in cluster %s", clusterID)
		}

		// Format users as options: "username [role]"
		var userOptions []string
		for _, user := range users {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
		}

		var userIndex int
		err = prompt.Select(ctx, &userIndex, "Select user to delete:", "", userOptions...)
		if err != nil {
			return err
		}

		username = users[userIndex].Name
	}

	if !flag.GetYes(ctx) {
		const msg = "Deleting a user is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Delete user %s from cluster %s?", username, clusterID); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("--yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	fmt.Fprintf(out, "Deleting user %s from cluster %s...\n", username, clusterID)

	err := flapsClient.DeleteManagedPostgresUser(ctx, clusterID, username)
	if flapsutil.IsFlapsRouteMissing(err) {
		err = mpgv2.ClientFromContext(ctx).DeleteUser(ctx, clusterID, username)
	}
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	fmt.Fprintf(out, "User %s deleted successfully from cluster %s\n", username, clusterID)

	return nil
}

func RunUsersRotatePassword(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	io := iostreams.FromContext(ctx)
	out := io.Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	username := flag.GetString(ctx, "username")
	if username == "" {
		if cfg.JSONOutput {
			return prompt.NonInteractiveError("username must be specified with --username flag when using --json")
		}
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}

		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}
		if len(users) == 0 {
			return fmt.Errorf("no users found in cluster %s", clusterID)
		}

		var userOptions []string
		for _, user := range users {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
		}

		var userIndex int
		if err := prompt.Select(ctx, &userIndex, "Select user to rotate:", "", userOptions...); err != nil {
			return err
		}
		username = users[userIndex].Name
	}

	killSessions := flag.GetBool(ctx, "kill-sessions")
	if !flag.GetYes(ctx) {
		fmt.Fprintln(io.ErrOut, "Rotating a password cannot be undone. Applications using the old password will fail to open new connections. Update their DATABASE_URL secrets after rotation.")
		if killSessions {
			fmt.Fprintln(io.ErrOut, "Existing database sessions will also be terminated.")
		}
		if cfg.JSONOutput {
			return prompt.NonInteractiveError("--yes flag must be specified when using --json")
		}
		switch confirmed, err := prompt.Confirmf(ctx, "Rotate password for user %s in cluster %s?", username, clusterID); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("--yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	if !cfg.JSONOutput {
		fmt.Fprintf(out, "Rotating password for user %s in cluster %s...\n", username, clusterID)
	}

	creds, err := flapsClient.RotateManagedPostgresUserPassword(ctx, clusterID, username, flaps.RotateManagedPostgresUserPasswordRequest{
		KillSessions: killSessions,
	})
	if err != nil {
		var flapsErr *flaps.FlapsError
		ambiguous := true
		if errors.As(err, &flapsErr) {
			if passwordRotatedWithoutTerminatingSessions(flapsErr) {
				fmt.Fprintln(io.ErrOut, "Note: the password was rotated, but existing sessions could not be terminated. Existing sessions may still be active and need to be terminated manually.")
				current, recoveryErr := flapsClient.GetManagedPostgresUserCredentials(ctx, clusterID, username)
				if recoveryErr != nil || current.Username != username || current.Password == "" {
					return fmt.Errorf("password for user %s was rotated, but sessions could not be terminated and current credentials could not be retrieved. Retrieve them from the dashboard Connect tab or the Get User Credentials API (https://docs.fly.io/api/machines/postgres-clusters/get-user-credentials). Another rotation will change the password again.", username)
				}
				if err := renderRotatedUserCredentials(out, current, cfg.JSONOutput, false, true); err != nil {
					return fmt.Errorf("password for user %s was rotated, but existing sessions could not be terminated and the current credentials could not be displayed: %w", username, err)
				}

				return fmt.Errorf("failed to terminate existing sessions after rotating password for user %s", username)
			}
			if flapsErr.ResponseStatusCode >= 400 && flapsErr.ResponseStatusCode < 500 {
				ambiguous = false
			}
		}
		if ambiguous {
			fmt.Fprintln(io.ErrOut, "Note: the password may already have been rotated. Check current credentials in the dashboard Connect tab or through the Get User Credentials API (https://docs.fly.io/api/machines/postgres-clusters/get-user-credentials) before considering another rotation.")
			if killSessions {
				fmt.Fprintln(io.ErrOut, "Note: with --kill-sessions, session termination is a separate step from rotation and may not have completed.")
			}
		}

		return fmt.Errorf("failed to rotate password for user %s: %w", username, err)
	}

	return renderRotatedUserCredentials(out, creds, cfg.JSONOutput, killSessions, false)
}

func renderRotatedUserCredentials(out stdio.Writer, creds flaps.ManagedPostgresUserCredentials, jsonOutput, sessionsTerminated, recovered bool) error {
	if jsonOutput {
		return render.JSON(out, creds)
	}
	header := "Password rotated successfully!"
	if recovered {
		header = "Current credentials:"
	}
	if _, err := fmt.Fprintf(out, "%s\n  Username: %s\n  Password: %s\n", header, creds.Username, creds.Password); err != nil {
		return fmt.Errorf("password for user %s was rotated but could not be displayed: %w", creds.Username, err)
	}
	if sessionsTerminated {
		_, err := fmt.Fprintf(out, "Existing sessions for %s were terminated.\n", creds.Username)
		return err
	}

	return nil
}

// Only this structured server error confirms that rotation completed. Reject
// oversized or invalid bodies in full; a truncated prefix cannot confirm it.
func passwordRotatedWithoutTerminatingSessions(err *flaps.FlapsError) bool {
	const maxErrorBody = 64 * 1024
	if err.ResponseStatusCode < 500 || err.ResponseStatusCode >= 600 || len(err.ResponseBody) > maxErrorBody {
		return false
	}
	status := err.StatusCode()

	return status != nil && *status == "sessions_not_terminated"
}
