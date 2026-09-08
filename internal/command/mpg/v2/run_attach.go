package cmdv2

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

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
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		users, err := listUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		var userOptions []string
		for _, user := range users {
			userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
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
			roleOptions := managedPostgresUserRoleOptions
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
		}
		// If no users found and create wasn't selected, username remains empty
		// and will use default credentials.
	}

	// Database selection priority: flag > prompt result (if interactive) > empty (use default credentials from cluster).
	db := flag.GetString(ctx, "database")
	if db == "" && io.IsInteractive() {
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

	// Get cluster details and default credentials. The public Machines API
	// cluster show does not bundle credentials the way the legacy
	// GetClusterById response does, so we assemble the equivalent here from
	// the pooler endpoint plus a public default-user credentials call. Falls
	// back to the legacy client only on a classified 404.
	info, err := getClusterConnectionInfoPublicFirst(ctx, flapsClient, legacyClient, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}
	if info.BaseURI == "" {
		return fmt.Errorf("cluster is not ready; cannot attach without valid connection information")
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
		user = info.DefaultUser
		password = info.DefaultPassword
	}

	if db == "" {
		db = info.DefaultDBName
	}
	connectionUri, err = buildConnectionUri(info.BaseURI, user, password, db)
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

// clusterConnectionInfo is the assembled view of what RunAttach needs from
// the cluster before building its DATABASE_URL: the base URI to substitute
// credentials into, plus the default user/password and the default database
// name to use when the caller did not specify --username / --database.
type clusterConnectionInfo struct {
	// BaseURI is the URI shape (scheme + host + port + default path) the
	// final connection URI is built from. Same shape as the legacy
	// Credentials.ConnectionUri, so buildConnectionUri can rewrite its
	// user and path components.
	BaseURI string
	// DefaultUser and DefaultPassword are the cluster's default credentials,
	// used only when the caller did not specify --username.
	DefaultUser     string
	DefaultPassword string
	// DefaultDBName is the cluster's default database name, used only when
	// the caller did not specify --database.
	DefaultDBName string
}

// getClusterConnectionInfoPublicFirst assembles the base URI and default
// credentials RunAttach uses to build DATABASE_URL. It tries the public
// Machines API first (cluster show + default-user credentials) and falls
// back to the legacy MPGv2 client only on a classified 404. This is the
// public-first equivalent of the unconditional legacy GetClusterById call
// that the attach flow used to rely on for the bundled Credentials bundle.
func getClusterConnectionInfoPublicFirst(ctx context.Context, flapsClient flapsutil.FlapsClient, legacyClient mpgv2.ClientV2, clusterID string) (clusterConnectionInfo, error) {
	cluster, err := flapsClient.GetManagedPostgresCluster(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		return getClusterConnectionInfoLegacy(ctx, legacyClient, clusterID)
	}
	if err != nil {
		return clusterConnectionInfo{}, err
	}

	// A missing pooler host cannot produce a usable URI. Stop before the
	// credentials lookup so a credentials 404 cannot bypass this guard.
	// Assigned endpoints may be available during initializing or creating;
	// endpoint presence does not establish cluster readiness.
	if cluster.Endpoints.Primary.Pooler.Host == "" {
		return clusterConnectionInfo{}, nil
	}

	creds, credsErr := flapsClient.GetManagedPostgresUserCredentials(ctx, clusterID, mpgutil.DefaultUsername)
	if errors.Is(credsErr, flaps.ErrFlapsNotFound) {
		// Public default-credentials endpoint missing; fall back to the
		// legacy bundle, which carries the base URI and default creds
		// together. Re-fetching the cluster here is wasteful but rare.
		return getClusterConnectionInfoLegacy(ctx, legacyClient, clusterID)
	}
	if credsErr != nil {
		return clusterConnectionInfo{}, credsErr
	}

	return clusterConnectionInfo{
		BaseURI:         buildBaseURIFromPublicCluster(cluster),
		DefaultUser:     creds.Username,
		DefaultPassword: creds.Password,
		DefaultDBName:   mpgutil.DefaultDatabase,
	}, nil
}

// getClusterConnectionInfoLegacy is the fallback path for
// getClusterConnectionInfoPublicFirst. It pulls the base URI and default
// credentials out of the legacy GetClusterById bundle.
func getClusterConnectionInfoLegacy(ctx context.Context, legacyClient mpgv2.ClientV2, clusterID string) (clusterConnectionInfo, error) {
	clusterResp, err := legacyClient.GetClusterById(ctx, clusterID)
	if err != nil {
		return clusterConnectionInfo{}, err
	}

	return clusterConnectionInfo{
		BaseURI:         clusterResp.Credentials.ConnectionUri,
		DefaultUser:     clusterResp.Credentials.User,
		DefaultPassword: clusterResp.Credentials.Password,
		DefaultDBName:   clusterResp.Credentials.DBName,
	}, nil
}

// buildBaseURIFromPublicCluster assembles a libpq connection URI from the
// public cluster's pooler endpoint and the established default database
// name, matching the shape of the legacy Credentials.ConnectionUri so
// buildConnectionUri can rewrite its user and path components.
//
// Returns an empty string when the pooler host is missing. This lets the
// caller reject incomplete connection information before writing a secret.
// Assigned endpoints can exist before the cluster reaches ready status.
func buildBaseURIFromPublicCluster(cluster flaps.ManagedPostgresCluster) string {
	host := cluster.Endpoints.Primary.Pooler.Host
	if host == "" {
		return ""
	}
	port := cluster.Endpoints.Primary.Pooler.Port
	if port == 0 {
		port = mpgutil.DefaultPort
	}

	return fmt.Sprintf("postgres://%s:%d/%s", host, port, mpgutil.DefaultDatabase)
}
