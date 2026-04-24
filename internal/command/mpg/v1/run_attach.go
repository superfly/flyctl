package cmdv1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func RunAttach(ctx context.Context, clusterID string, app *fly.AppBasic) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
	)

	mpgClient := mpgv1.ClientFromContext(ctx)

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		usersResponse, err := mpgClient.ListUsers(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		var userOptions []string
		for _, user := range usersResponse.Data {
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
			var userName string
			err = prompt.String(ctx, &userName, "Enter username:", "", true)
			if err != nil {
				return err
			}
			if userName == "" {
				return fmt.Errorf("username cannot be empty")
			}

			// Prompt for role selection
			var roleIndex int
			roleOptions := []string{"schema_admin", "writer", "reader"}
			err = prompt.Select(ctx, &roleIndex, "Select user role:", "", roleOptions...)
			if err != nil {
				return err
			}
			userRole := roleOptions[roleIndex]

			fmt.Fprintf(io.Out, "Creating user %s with role %s...\n", userName, userRole)

			input := mpgv1.CreateUserWithRoleInput{
				UserName: userName,
				Role:     userRole,
			}

			createResponse, err := mpgClient.CreateUserWithRole(ctx, clusterID, input)
			if err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			fmt.Fprintf(io.Out, "User created successfully!\n")
			username = createResponse.Data.Name
		} else if len(usersResponse.Data) > 0 {
			username = usersResponse.Data[userIndex].Name
		}
		// If no users found and create wasn't selected, username remains empty and will use default credentials.
		// This shouldn't be hit as fly-db and fly-user always exist and can't be deleted.
	}

	// Database selection priority: flag > prompt result (if interactive) > credentials.DBName
	var db string
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
	} else if io.IsInteractive() {
		// Prompt for database selection
		databasesResponse, err := mpgClient.ListDatabases(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		var dbOptions []string
		for _, database := range databasesResponse.Data {
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

			input := mpgv1.CreateDatabaseInput{
				Name: dbName,
			}

			createResponse, err := mpgClient.CreateDatabase(ctx, clusterID, input)
			if err != nil {
				return fmt.Errorf("failed to create database: %w", err)
			}

			fmt.Fprintf(io.Out, "Database created successfully!\n")
			db = createResponse.Data.Name
		} else if len(databasesResponse.Data) > 0 {
			db = databasesResponse.Data[dbIndex].Name
		}
	}

	// Get cluster details with credentials
	response, err := mpgClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	// Get credentials - use user-specific endpoint if username provided, otherwise use default
	var credentials mpgv1.GetManagedClusterCredentialsResponse
	if username != "" {
		userCreds, err := mpgClient.GetUserCredentials(ctx, clusterID, username)
		if err != nil {
			return fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}
		// Convert user credentials to the standard format
		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Data.User,
			Password: userCreds.Data.Password,
			DBName:   response.Credentials.DBName, // Use default DB name from cluster credentials
		}
	} else {
		credentials = response.Credentials
	}

	// Use selected database or fall back to default from credentials
	if db == "" {
		db = credentials.DBName
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	variableName := flag.GetString(ctx, "variable-name")

	if variableName == "" {
		variableName = "DATABASE_URL"
	}

	// Check if the app already has the secret variable set
	secrets, err := appsecrets.List(ctx, flapsClient, app.Name)
	if err != nil {
		return fmt.Errorf("failed retrieving secrets for app %s: %w", appName, err)
	}

	for _, secret := range secrets {
		if secret.Name == variableName {
			return fmt.Errorf("app %s already has %s set. Use 'fly secrets unset %s' to remove it first", appName, variableName, variableName)
		}
	}

	// Build connection URI with selected user and database
	// Parse the base connection URI to extract host/port
	baseUri := response.Credentials.ConnectionUri
	parsedUri, err := url.Parse(baseUri)
	if err != nil {
		return fmt.Errorf("failed to parse connection URI: %w", err)
	}

	// Build new connection URI with selected user, password, and database
	parsedUri.User = url.UserPassword(credentials.User, credentials.Password)
	parsedUri.Path = "/" + db
	connectionUri := parsedUri.String()

	s := map[string]string{}
	s[variableName] = connectionUri

	if err := appsecrets.Update(ctx, flapsClient, app.Name, s, nil); err != nil {
		return err
	}

	// Create attachment record to track the cluster-app relationship
	attachInput := mpgv1.CreateAttachmentInput{
		AppName: appName,
	}
	if _, err := mpgClient.CreateAttachment(ctx, clusterID, attachInput); err != nil {
		// Log warning but don't fail - the secret was set successfully
		fmt.Fprintf(io.ErrOut, "Warning: failed to create attachment record: %v\n", err)
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is being attached to %s\n", clusterID, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, variableName, connectionUri)

	return nil
}
