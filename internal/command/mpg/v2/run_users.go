package cmdv2

import (
	"context"
	"errors"
	"fmt"
	stdio "io"
	"net/http"
	"slices"
	"strings"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
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
	string(flaps.ManagedPostgresUserRoleCustom),
}

// roleOptionLabels labels roles for a prompt. custom says it grants nothing,
// since that surprises people.
func roleOptionLabels(roles []string) []string {
	labels := make([]string, len(roles))
	for i, role := range roles {
		labels[i] = role
		if role == string(flaps.ManagedPostgresUserRoleCustom) {
			labels[i] = role + " (no access until granted with SQL)"
		}
	}

	return labels
}

func validateUserRole(role string) error {
	if slices.Contains(managedPostgresUserRoleOptions, role) {
		return nil
	}

	return fmt.Errorf("invalid role %q. Must be one of: %s", role, strings.Join(managedPostgresUserRoleOptions, ", "))
}

// promptUserRole asks for a role when none was given, and checks it otherwise.
func promptUserRole(ctx context.Context, role string) (string, error) {
	if role != "" {
		return role, validateUserRole(role)
	}
	if !iostreams.FromContext(ctx).IsInteractive() {
		return "", prompt.NonInteractiveError("user role must be specified with --role flag when not running interactively")
	}

	var roleIndex int
	if err := prompt.Select(ctx, &roleIndex, "Select user role:", "", roleOptionLabels(managedPostgresUserRoleOptions)...); err != nil {
		return "", err
	}

	return managedPostgresUserRoleOptions[roleIndex], nil
}

const clusterUpdateHint = "It arrives with a scheduled update in the cluster's maintenance window."

// withClusterUpdateHint explains a 501: the cluster is too old for the call.
func withClusterUpdateHint(err error) error {
	if errors.Is(err, flaps.ErrFlapsNotImplemented) {
		return flyerr.WithSuggestion(err, clusterUpdateHint)
	}

	return err
}

const (
	allDatabasesOption  = "All databases, including new ones"
	someDatabasesOption = "Only some databases"
)

// promptUserDatabases asks which databases a custom user can connect to. It
// returns nil for every database. current is preselected.
func promptUserDatabases(ctx context.Context, clusterID string, current []string) ([]string, error) {
	def := allDatabasesOption
	if len(current) > 0 {
		def = someDatabasesOption
	}

	var choice int
	if err := prompt.Select(ctx, &choice, "Which databases can the user connect to?", def, allDatabasesOption, someDatabasesOption); err != nil {
		return nil, err
	}
	if choice == 0 {
		return nil, nil
	}

	databases, err := listDatabasesPublicFirst(ctx, flapsutil.ClientFromContext(ctx), mpgv2.ClientFromContext(ctx), clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	if len(databases) == 0 {
		return nil, fmt.Errorf("no databases found in cluster %s", clusterID)
	}

	names := make([]string, 0, len(databases))
	var preselected []int
	for i, database := range databases {
		names = append(names, database.Name)
		if slices.Contains(current, database.Name) {
			preselected = append(preselected, i)
		}
	}

	var picked []int
	if err := prompt.MultiSelect(ctx, &picked, "Select databases:", preselected, names...); err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, errors.New("select at least one database")
	}

	selected := make([]string, 0, len(picked))
	for _, i := range picked {
		selected = append(selected, names[i])
	}

	return selected, nil
}

// databasesFlag returns --database, or nil when it isn't given. The API
// rejects an empty list, so nil is what means every database.
func databasesFlag(ctx context.Context) []string {
	databases := flag.GetStringArray(ctx, "database")
	if len(databases) == 0 {
		return nil
	}

	return databases
}

// publicUserToLegacy converts the public response to the legacy output shape.
func publicUserToLegacy(u flaps.ManagedPostgresUser) mpgv2.User {
	return mpgv2.User{
		Name:             u.Username,
		Role:             string(u.Role),
		Roles:            u.Roles,
		DatabasesLimited: u.DatabasesLimited,
		Databases:        u.Databases,
	}
}

// userRolesCell shows a user's customer roles. It is blank when the cluster
// doesn't report them, and "-" when the user has none.
func userRolesCell(roles []string) string {
	if roles == nil {
		return ""
	}
	if len(roles) == 0 {
		return "-"
	}

	return strings.Join(roles, ", ")
}

// userDatabasesCell shows which databases a user can reach. Users without a
// built-in role are reported as not limited, but their access comes from SQL
// grants, so it shows "unknown" for them.
func userDatabasesCell(limited bool, databases []string, role string) string {
	switch {
	case limited && len(databases) == 0:
		return "none"
	case limited:
		return strings.Join(databases, ", ")
	case role == "":
		return "unknown"
	default:
		return "all"
	}
}

func listUsers(ctx context.Context, clusterID string) ([]mpgv2.User, error) {
	publicUsers, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresUsers(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
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
	io := iostreams.FromContext(ctx)
	out := io.Out
	users, err := listUsers(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list users for cluster %s: %w", clusterID, err)
	}

	if len(users) == 0 {
		fmt.Fprintf(out, "No users found for cluster %s\n", clusterID)

		return nil
	}

	if cfg.JSONOutput {
		// roles stays null when not reported; databases is always a list.
		for i := range users {
			if users[i].Databases == nil {
				users[i].Databases = []string{}
			}
		}

		return render.JSON(out, users)
	}

	rolesReported := true
	rows := make([][]string, 0, len(users))
	for _, user := range users {
		if user.Roles == nil {
			rolesReported = false
		}
		rows = append(rows, []string{
			user.Name,
			user.Role,
			userRolesCell(user.Roles),
			userDatabasesCell(user.DatabasesLimited, user.Databases, user.Role),
		})
	}

	if err := render.Table(out, "", rows, "Name", "Role", "Roles", "Databases"); err != nil {
		return err
	}
	if !rolesReported {
		fmt.Fprintln(io.ErrOut, "This cluster doesn't report customer roles yet.")
	}

	return nil
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

	role, err := promptUserRole(ctx, flag.GetString(ctx, "role"))
	if err != nil {
		return err
	}

	isCustom := role == string(flaps.ManagedPostgresUserRoleCustom)
	databases := databasesFlag(ctx)
	if databases != nil && !isCustom {
		return errors.New("--database needs --role custom: only custom users can be limited to databases")
	}
	if isCustom && databases == nil && iostreams.FromContext(ctx).IsInteractive() {
		if databases, err = promptUserDatabases(ctx, clusterID, nil); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Creating user %s with role %s in cluster %s...\n", username, role, clusterID)

	input := flaps.CreateManagedPostgresUserRequest{
		Username:  username,
		Role:      role,
		Databases: databases,
	}

	response, err := flapsClient.CreateManagedPostgresUser(ctx, clusterID, input)
	// postgresv2 is only a fallback for users with every database: a server
	// that drops the list there would create an unlimited user.
	if errors.Is(err, flaps.ErrFlapsNotFound) && databases == nil {
		legacyInput := mpgv2.CreateUserWithRoleInput{Username: input.Username, Role: input.Role}
		legacyResponse, legacyErr := mpgv2.ClientFromContext(ctx).CreateUserWithRole(ctx, clusterID, legacyInput)
		if legacyErr != nil {
			return fmt.Errorf("failed to create user: %w", legacyErr)
		}
		response = flaps.ManagedPostgresUser{
			Username: legacyResponse.Data.Name,
			Role:     flaps.ManagedPostgresUserRole(legacyResponse.Data.Role),
		}
	} else if err != nil {
		return fmt.Errorf("failed to create user: %w", withClusterUpdateHint(err))
	}

	fmt.Fprintf(out, "User created successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", response.Username)
	fmt.Fprintf(out, "  Role: %s\n", string(response.Role))
	fmt.Fprintf(out, "  Databases: %s\n", userDatabasesCell(response.DatabasesLimited, response.Databases, string(response.Role)))
	if isCustom {
		fmt.Fprintln(out, "custom users can't read or write anything until an admin user grants access with SQL.")
	}

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

	role, err := promptUserRole(ctx, flag.GetString(ctx, "role"))
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Updating user %s role to %s in cluster %s...\n", username, role, clusterID)

	input := flaps.UpdateManagedPostgresUserRoleRequest{
		Role: role,
	}

	err = flapsClient.UpdateManagedPostgresUserRole(ctx, clusterID, username, input)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		err = mpgv2.ClientFromContext(ctx).UpdateUserRole(ctx, clusterID, username, mpgv2.UpdateUserRoleInput(input))
	}
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", withClusterUpdateHint(err))
	}

	fmt.Fprintf(out, "User role updated successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", username)
	fmt.Fprintf(out, "  Role: %s\n", role)

	return nil
}

// userDatabasesLabel describes a user's databases in a picker.
func userDatabasesLabel(user mpgv2.User) string {
	switch {
	case !user.DatabasesLimited:
		return "all databases"
	case len(user.Databases) == 0:
		return "no databases"
	default:
		return strings.Join(user.Databases, ", ")
	}
}

// selectCustomUser asks for one of the cluster's custom users, the only ones
// that can be limited to databases.
func selectCustomUser(ctx context.Context, clusterID string) (mpgv2.User, error) {
	users, err := listUsers(ctx, clusterID)
	if err != nil {
		return mpgv2.User{}, fmt.Errorf("failed to list users: %w", err)
	}

	var customUsers []mpgv2.User
	var options []string
	for _, user := range users {
		if user.Role != string(flaps.ManagedPostgresUserRoleCustom) {
			continue
		}
		customUsers = append(customUsers, user)
		options = append(options, fmt.Sprintf("%s [%s]", user.Name, userDatabasesLabel(user)))
	}
	if len(customUsers) == 0 {
		return mpgv2.User{}, fmt.Errorf("no custom users in cluster %s; create one with fly mpg users create --role custom", clusterID)
	}

	var index int
	if err := prompt.Select(ctx, &index, "Select user:", "", options...); err != nil {
		return mpgv2.User{}, err
	}

	return customUsers[index], nil
}

// currentUserDatabases returns the databases a limited user can reach, to
// preselect them in the prompt.
func currentUserDatabases(ctx context.Context, clusterID, username string) ([]string, error) {
	users, err := listUsers(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	for _, user := range users {
		if user.Name == username && user.DatabasesLimited {
			return user.Databases, nil
		}
	}

	return nil, nil
}

// setDatabasesMayHaveApplied reports whether a failed set-databases call may
// still have changed the user. The server applies the change before it ends
// the user's sessions, so a server error can come after the change.
func setDatabasesMayHaveApplied(err error) bool {
	var flapsErr *flaps.FlapsError
	if !errors.As(err, &flapsErr) {
		return true
	}

	return flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode != http.StatusNotImplemented
}

// RunUsersSetDatabases limits a custom user to some databases, or gives it
// every database. It calls the Machines API only.
func RunUsersSetDatabases(ctx context.Context, clusterID string) error {
	io := iostreams.FromContext(ctx)
	out := io.Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	databases := databasesFlag(ctx)
	allDatabases := flag.GetBool(ctx, "all-databases")
	if databases != nil && allDatabases {
		return errors.New("use either --database or --all-databases")
	}
	if databases == nil && !allDatabases && !io.IsInteractive() {
		return prompt.NonInteractiveError("--database or --all-databases must be specified when not running interactively")
	}

	username := flag.GetString(ctx, "username")
	var current []string
	if username == "" {
		if !io.IsInteractive() {
			return prompt.NonInteractiveError("username must be specified with --username flag when not running interactively")
		}
		user, err := selectCustomUser(ctx, clusterID)
		if err != nil {
			return err
		}
		username = user.Name
		if user.DatabasesLimited {
			current = user.Databases
		}
	} else if databases == nil && !allDatabases {
		var err error
		if current, err = currentUserDatabases(ctx, clusterID, username); err != nil {
			return err
		}
	}

	if databases == nil && !allDatabases {
		var err error
		if databases, err = promptUserDatabases(ctx, clusterID, current); err != nil {
			return err
		}
	}

	req := flaps.SetManagedPostgresUserDatabasesRequest{
		DatabasesLimited: databases != nil,
		Databases:        databases,
	}

	// Only limiting ends sessions, so only limiting asks first.
	if req.DatabasesLimited && !flag.GetYes(ctx) {
		switch confirmed, err := prompt.Confirmf(ctx, "Limit %s to %s in cluster %s? Its sessions in other databases will be ended.", username, strings.Join(databases, ", "), clusterID); {
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

	fmt.Fprintf(out, "Updating databases for user %s in cluster %s...\n", username, clusterID)

	if err := flapsClient.SetManagedPostgresUserDatabases(ctx, clusterID, username, req); err != nil {
		if setDatabasesMayHaveApplied(err) {
			fmt.Fprintln(io.ErrOut, "Note: the change may already be in place. It is safe to run the same command again.")
			if req.DatabasesLimited {
				fmt.Fprintln(io.ErrOut, "Sessions in other databases may still be open; running the command again ends them.")
			}
		}

		return fmt.Errorf("failed to update databases for user %s: %w", username, withClusterUpdateHint(err))
	}

	fmt.Fprintf(out, "User databases updated successfully!\n")
	fmt.Fprintf(out, "  Name: %s\n", username)
	fmt.Fprintf(out, "  Databases: %s\n", userDatabasesCell(req.DatabasesLimited, req.Databases, string(flaps.ManagedPostgresUserRoleCustom)))

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
	if errors.Is(err, flaps.ErrFlapsNotFound) {
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
