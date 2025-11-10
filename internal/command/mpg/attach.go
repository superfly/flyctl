package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
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
			Name:        "database-name",
			Description: "The name of the database to create. Defaults to the app name.",
		},
		flag.String{
			Name:        "database-user",
			Description: "The name of the database user to create. Defaults to the app name.",
		},
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The name of the environment variable that will be added to the attached app",
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

	response, err := uiexClient.GetManagedClusterById(ctx, cluster.Id)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	ctx, flapsClient, _, err := flapsutil.SetClient(ctx, nil, appName)
	if err != nil {
		return err
	}

	var (
		databaseName = flag.GetString(ctx, "database-name")
		databaseUser = flag.GetString(ctx, "database-user")
		variableName = flag.GetString(ctx, "variable-name")
	)

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

	// Determine connection string based on whether custom database/user was requested
	var connectionUri string

	if databaseName != "" || databaseUser != "" {
		// Custom database/user specified - create them via the API
		if databaseName == "" {
			databaseName = appName
		}
		if databaseUser == "" {
			databaseUser = appName
		}

		fmt.Fprintf(io.Out, "Creating database %s and user %s\n", databaseName, databaseUser)

		createUserResp, err := uiexClient.CreateUser(ctx, clusterId, uiex.CreateUserInput{
			DbName:   databaseName,
			UserName: databaseUser,
			AppName:  appName,
		})
		if err != nil {
			return fmt.Errorf("failed to create database and user: %w", err)
		}

		if !createUserResp.Ok {
			return fmt.Errorf("failed to create database and user: %s", createUserResp.Errors.Detail)
		}

		connectionUri = createUserResp.ConnectionUri
	} else {
		// No custom database/user - use default connection string
		connectionUri = response.Credentials.ConnectionUri
	}

	s := map[string]string{}
	s[variableName] = connectionUri

	if err := appsecrets.Update(ctx, flapsClient, app.Name, s, nil); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is now attached to %s\n", clusterId, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, variableName, connectionUri)

	return nil
}
