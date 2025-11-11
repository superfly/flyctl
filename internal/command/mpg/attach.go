package mpg

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach a managed Postgres cluster to an app"
		long  = short + ". " +
			`This command will add a secret to the specified app
 containing the connection string for the database.`
		usage = "attach <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
		command.RequireUiex,
	)
	// cmd.Args = cobra.ExactArgs(1)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The name of the environment variable that will be added to the attached app",
		},
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to connect as",
		},
	)

	return cmd
}

func runAttach(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	var (
		clusterId = flag.FirstArg(ctx)
		appName   = appconfig.NameFromContext(ctx)
		client    = flyutil.ClientFromContext(ctx)
		io        = iostreams.FromContext(ctx)
	)

	// Get app details to determine which org it belongs to
	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	appOrgSlug := app.Organization.RawSlug
	if appOrgSlug != "" && clusterId == "" {
		fmt.Fprintf(io.Out, "Listing clusters in organization %s\n", appOrgSlug)
	}

	// Get cluster details to determine which org it belongs to
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterId, appOrgSlug)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	clusterOrgSlug := cluster.Organization.Slug

	// Verify that the app and cluster are in the same organization
	if appOrgSlug != clusterOrgSlug {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization to attach",
			appName, appOrgSlug, cluster.Id, clusterOrgSlug)
	}

	uiexClient := uiexutil.ClientFromContext(ctx)

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		usersResponse, err := uiexClient.ListUsers(ctx, cluster.Id)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(usersResponse.Data) > 0 {
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
		// If no users found, username remains empty and will use default credentials
	}

	// Database selection priority: flag > prompt result (if interactive) > credentials.DBName
	var db string
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
	} else if io.IsInteractive() {
		// Prompt for database selection
		databasesResponse, err := uiexClient.ListDatabases(ctx, cluster.Id)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		if len(databasesResponse.Data) > 0 {
			var dbOptions []string
			for _, database := range databasesResponse.Data {
				dbOptions = append(dbOptions, database.Name)
			}

			var dbIndex int
			err = prompt.Select(ctx, &dbIndex, "Select database:", "", dbOptions...)
			if err != nil {
				return err
			}

			db = databasesResponse.Data[dbIndex].Name
		}
	}

	// Get cluster details with credentials
	response, err := uiexClient.GetManagedClusterById(ctx, cluster.Id)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	// Get credentials - use user-specific endpoint if username provided, otherwise use default
	var credentials uiex.GetManagedClusterCredentialsResponse
	if username != "" {
		userCreds, err := uiexClient.GetUserCredentials(ctx, cluster.Id, username)
		if err != nil {
			return fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}
		// Convert user credentials to the standard format
		credentials = uiex.GetManagedClusterCredentialsResponse{
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

	ctx, flapsClient, _, err := flapsutil.SetClient(ctx, nil, appName)
	if err != nil {
		return err
	}

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

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is being attached to %s\n", cluster.Id, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, variableName, connectionUri)

	return nil
}
