package cmdv2

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// attachUserRoleOptions are the roles attach offers for a new user. custom
// is left out: a new custom user can't read anything, so its DATABASE_URL
// would be of no use.
var attachUserRoleOptions = []string{
	string(flaps.ManagedPostgresUserRoleSchemaAdmin),
	string(flaps.ManagedPostgresUserRoleWriter),
	string(flaps.ManagedPostgresUserRoleReader),
}

// RunAttach attaches a managed Postgres cluster to an app.
func RunAttach(ctx context.Context, clusterID string) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
	)

	flapsClient := flapsutil.ClientFromContext(ctx)
	legacyClient := mpgv2.ClientFromContext(ctx)

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	// limitedUser is set when the chosen user is limited to some databases.
	// Only a users list that reports databases_limited true marks a user as
	// limited. A list from an API without database limits, such as the
	// postgresv2 fallback on older deployments, leaves the field out, so it
	// decodes to false and no user is checked.
	var limitedUser *mpgv2.User
	if username != "" {
		// Fail closed: without the list, attach can't tell whether the URL
		// would connect.
		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("couldn't check %s's database limits: %w", username, err)
		}
		if i := slices.IndexFunc(users, func(user mpgv2.User) bool { return user.Name == username }); i >= 0 && users[i].DatabasesLimited {
			limitedUser = &users[i]
		}
	} else if io.IsInteractive() {
		// Prompt for user selection
		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		var userOptions []string
		for _, user := range users {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, userRoleLabel(user.Role)))
		}
		// Add option to create new user
		userOptions = append(userOptions, "Create new user...")

		var userIndex int
		err = prompt.Select(ctx, &userIndex, "Select user:", "", userOptions...)
		if err != nil {
			return err
		}

		if userIndex == len(userOptions)-1 {
			// Create new user option selected
			var userInput string
			err = prompt.String(ctx, &userInput, "Enter username:", "", true)
			if err != nil {
				return err
			}
			if userInput == "" {
				return fmt.Errorf("username cannot be empty")
			}

			// Prompt for role selection
			var roleIndex int
			roleOptions := attachUserRoleOptions
			err = prompt.Select(ctx, &roleIndex, "Select user role:", "", roleOptions...)
			if err != nil {
				return err
			}
			role := roleOptions[roleIndex]

			fmt.Fprintf(io.Out, "Creating user %s with role %s...\n", userInput, role)

			user, err := createUserPublicFirst(ctx, flapsClient, legacyClient, clusterID, userInput, role)
			if err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			fmt.Fprintf(io.Out, "User created successfully!\n")
			username = user.Name
		} else if len(users) > 0 {
			username = users[userIndex].Name
			if users[userIndex].DatabasesLimited {
				limitedUser = &users[userIndex]
			}
		}
		// If no users found and create wasn't selected, username remains empty
		// and will use default credentials.
	}

	// Database selection priority: flag > prompt result (if interactive) > empty (use default credentials from cluster).
	db := flag.GetString(ctx, "database")
	if limitedUser != nil && len(limitedUser.Databases) == 0 {
		return fmt.Errorf("user %s can't connect to any database; give it some with fly mpg users set-databases", limitedUser.Name)
	}
	if db == "" && io.IsInteractive() && limitedUser != nil {
		// Only the user's own databases; a new one would be out of its reach.
		var dbIndex int
		if err := prompt.Select(ctx, &dbIndex, "Select database:", "", limitedUser.Databases...); err != nil {
			return err
		}
		db = limitedUser.Databases[dbIndex]
	} else if db == "" && io.IsInteractive() {
		// Prompt for database selection
		databases, err := listDatabasesPublicFirst(ctx, flapsClient, legacyClient, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		var dbOptions []string
		for _, database := range databases {
			dbOptions = append(dbOptions, database.Name)
		}
		// Add option to create new database
		dbOptions = append(dbOptions, "Create new database...")

		var dbIndex int
		err = prompt.Select(ctx, &dbIndex, "Select database:", "", dbOptions...)
		if err != nil {
			return err
		}

		if dbIndex == len(dbOptions)-1 {
			// Create new database option selected
			var dbName string
			err = prompt.String(ctx, &dbName, "Enter database name:", "", true)
			if err != nil {
				return err
			}
			if dbName == "" {
				return fmt.Errorf("database name cannot be empty")
			}

			fmt.Fprintf(io.Out, "Creating database %s...\n", dbName)

			err = createDatabasePublicFirst(ctx, flapsClient, legacyClient, clusterID, dbName)
			if err != nil {
				return fmt.Errorf("failed to create database: %w", err)
			}

			fmt.Fprintf(io.Out, "Database created successfully!\n")
			db = dbName
		} else if len(databases) > 0 {
			db = databases[dbIndex].Name
		}
	}

	// Get cluster details and credentials. The public Machines API cluster show does
	// not expose credentials, so we always use the legacy client for the connection
	// URI and default DB name. This preserves the existing behavior.
	clusterResp, err := legacyClient.GetClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	baseUri := clusterResp.Credentials.ConnectionUri
	if baseUri == "" {
		return fmt.Errorf("connection URI is empty; cannot attach without valid credentials")
	}

	var connectionUri string
	var user, password string

	if username != "" {
		creds, err := getUserCredentialsPublicFirst(ctx, flapsClient, legacyClient, clusterID, username)
		if err != nil {
			return fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}
		user = creds.User
		password = creds.Password
	} else {
		user = clusterResp.Credentials.User
		password = clusterResp.Credentials.Password
	}

	if db == "" {
		db = clusterResp.Credentials.DBName
	}
	if limitedUser != nil && !slices.Contains(limitedUser.Databases, db) {
		return flyerr.WithSuggestion(
			fmt.Errorf("user %s can't connect to database %s; it is limited to %s", limitedUser.Name, db, strings.Join(limitedUser.Databases, ", ")),
			"Pick one of those with --database, or change its databases with fly mpg users set-databases.",
		)
	}
	connectionUri, err = buildConnectionUri(baseUri, user, password, db)
	if err != nil {
		return fmt.Errorf("failed to build connection URI: %w", err)
	}

	variableName := flag.GetString(ctx, "variable-name")
	if variableName == "" {
		variableName = "DATABASE_URL"
	}

	// Check if the app already has the secret variable set
	secrets, err := appsecrets.List(ctx, flapsClient, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving secrets for app %s: %w", appName, err)
	}

	for _, secret := range secrets {
		if secret.Name == variableName {
			return fmt.Errorf("app %s already has %s set. Use 'fly secrets unset %s' to remove it first", appName, variableName, variableName)
		}
	}

	// Write the secret.
	s := map[string]string{}
	s[variableName] = connectionUri

	if err := appsecrets.Update(ctx, flapsClient, appName, s, nil); err != nil {
		return err
	}

	// Create attachment record to track the cluster-app relationship.
	attachInput := mpgv2.CreateAttachmentInput{
		AppName: appName,
	}
	err = createAttachmentPublicFirst(ctx, flapsClient, legacyClient, clusterID, attachInput)
	if err != nil {
		// Attachment is warning-only; the secret was set successfully.
		fmt.Fprintf(io.ErrOut, "Warning: failed to create attachment record: %v\n", err)
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is being attached to %s\n", clusterID, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, variableName, connectionUri)

	return nil
}

// createUserPublicFirst tries the public Machines API for user creation and
// falls back to the legacy MPGv2 client on a classified 404.
func createUserPublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID, username, role string) (mpgv2.User, error) {
	req := flaps.CreateManagedPostgresUserRequest{
		Username: username,
		Role:     role,
	}

	created, err := flapsClient.CreateManagedPostgresUser(ctx, clusterID, req)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		input := mpgv2.CreateUserWithRoleInput{
			Username: username,
			Role:     role,
		}
		response, legacyErr := legacyClient.CreateUserWithRole(ctx, clusterID, input)
		if legacyErr != nil {
			return mpgv2.User{}, legacyErr
		}

		return mpgv2.User{Name: response.Data.Name, Role: response.Data.Role}, nil
	}
	if err != nil {
		return mpgv2.User{}, err
	}

	return mpgv2.User{Name: created.Username, Role: string(created.Role)}, nil
}

// userCredentials is the internal shape used to carry user credentials through
// the attach flow, compatible with the legacy response fields used elsewhere.
type userCredentials struct {
	User     string
	Password string
}

// getUserCredentialsPublicFirst tries the public Machines API for user
// credentials and falls back to the legacy MPGv2 client on a classified 404.
func getUserCredentialsPublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID, username string) (userCredentials, error) {
	creds, err := flapsClient.GetManagedPostgresUserCredentials(ctx, clusterID, username)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		response, legacyErr := legacyClient.GetUserCredentials(ctx, clusterID, username)
		if legacyErr != nil {
			return userCredentials{}, legacyErr
		}

		return userCredentials{User: response.Data.User, Password: response.Data.Password}, nil
	}
	if err != nil {
		return userCredentials{}, err
	}

	return userCredentials{User: creds.Username, Password: creds.Password}, nil
}

// listDatabasesPublicFirst tries the public Machines API for database listing
// and falls back to the legacy MPGv2 client on a classified 404.
func listDatabasesPublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID string) ([]mpgv2.Database, error) {
	databases, err := flapsClient.ListManagedPostgresDatabases(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		response, legacyErr := legacyClient.ListDatabases(ctx, clusterID)
		if legacyErr != nil {
			return nil, legacyErr
		}

		return response.Data, nil
	}
	if err != nil {
		return nil, err
	}

	dbs := make([]mpgv2.Database, 0, len(databases))
	for _, d := range databases {
		dbs = append(dbs, mpgv2.Database{Name: d.Name})
	}

	return dbs, nil
}

// createDatabasePublicFirst tries the public Machines API for database
// creation and falls back to the legacy MPGv2 client on a classified 404.
func createDatabasePublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID, dbName string) error {
	req := flaps.CreateManagedPostgresDatabaseRequest{Name: dbName}

	_, err := flapsClient.CreateManagedPostgresDatabase(ctx, clusterID, req)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		return legacyClient.CreateDatabase(ctx, clusterID, mpgv2.CreateDatabaseInput{Name: dbName})
	}

	return err
}

// buildConnectionUri parses the base URI and substitutes user, password, and
// database to produce a complete connection string.
func buildConnectionUri(baseUri, user, password, db string) (string, error) {
	parsedURI, err := url.Parse(baseUri)
	if err != nil {
		return "", err
	}
	parsedURI.User = url.UserPassword(user, password)
	parsedURI.Path = "/" + db

	return parsedURI.String(), nil
}

// createAttachmentPublicFirst tries the public Machines API for attachment
// creation and falls back to the legacy MPGv2 client on a classified 404.
func createAttachmentPublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID string, input mpgv2.CreateAttachmentInput) error {
	req := flaps.CreateManagedPostgresAttachmentRequest{
		AppName: input.AppName,
	}

	_, err := flapsClient.CreateManagedPostgresAttachment(ctx, clusterID, req)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		_, err := legacyClient.CreateAttachment(ctx, clusterID, input)

		return err
	}
	if err != nil {
		return err
	}

	return nil
}
