package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The name of the environment variable that will be added to the attached app",
		},
	)

	return cmd
}

func runAttach(ctx context.Context) error {
	var (
		clusterId  = flag.FirstArg(ctx)
		appName    = appconfig.NameFromContext(ctx)
		client     = flyutil.ClientFromContext(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		io         = iostreams.FromContext(ctx)
	)

	// Get cluster details to determine which org it belongs to
	response, err := uiexClient.GetManagedClusterById(ctx, clusterId)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	clusterOrgSlug := response.Data.Organization.Slug

	// Get app details to determine which org it belongs to
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	appOrgSlug := app.Organization.Slug

	// Verify that the app and cluster are in the same organization
	if appOrgSlug != clusterOrgSlug {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization to attach",
			appName, appOrgSlug, clusterId, clusterOrgSlug)
	}

	variableName := flag.GetString(ctx, "variable-name")

	if variableName == "" {
		variableName = "DATABASE_URL"
	}

	// Check if the app already has the secret variable set
	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving secrets for app %s: %w", appName, err)
	}

	for _, secret := range secrets {
		if secret.Name == variableName {
			return fmt.Errorf("app %s already has %s set. Use 'fly secrets unset %s' to remove it first", appName, variableName, variableName)
		}
	}

	s := map[string]string{}
	s[variableName] = response.Credentials.ConnectionUri

	_, err = client.SetSecrets(ctx, app.Name, s)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is being attached to %s\n", clusterId, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, variableName, response.Credentials.ConnectionUri)

	return nil
}
