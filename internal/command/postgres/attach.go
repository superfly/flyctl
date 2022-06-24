package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/flypg"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach a postgres cluster to an app"
		long  = short + "\n"
		usage = "attach"
	)

	cmd := command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		// flag.String{
		// 	Name:        "postgres-app",
		// 	Description: "The name of the postgres app we are looking to attach.",
		// },
		flag.String{
			Name:        "database-name",
			Description: "The designated database name for this consuming app.",
		},
		flag.String{
			Name:        "database-user",
			Description: "The database user to create. By default, we will use the name of the consuming app.",
		},
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The environment variable name that will be added to the consuming app. ",
		},
	)

	return cmd
}

func runAttach(ctx context.Context) error {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion = "0.0.19"
		appName              = app.NameFromContext(ctx)
		pgAppName            = flag.FirstArg(ctx)
		client               = client.FromContext(ctx).API()
	)

	dbName := flag.GetString(ctx, "database-name")
	if dbName == "" {
		dbName = appName
	}
	dbName = strings.ToLower(strings.ReplaceAll(dbName, "-", "_"))

	dbUser := flag.GetString(ctx, "database-user")
	if dbUser == "" {
		dbUser = appName
	}
	dbUser = strings.ToLower(strings.ReplaceAll(dbUser, "-", "_"))

	varName := flag.GetString(ctx, "variable-name")
	if varName == "" {
		varName = "DATABASE_URL"
	}

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: pgAppName,
		ManualEntry:          true,
		DatabaseName:         api.StringPointer(dbName),
		DatabaseUser:         api.StringPointer(dbUser),
		VariableName:         api.StringPointer(varName),
	}

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgApp, err := client.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	switch app.PlatformVersion {
	case "nomad":
		if err := hasRequiredVersionOnNomad(pgApp, MinPostgresHaVersion, ""); err != nil {
			return err
		}
	case "machines":
		if err := hasRequiredVersionOnMachines(); err != nil {
			return err
		}
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, pgApp.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", pgApp.Organization.Slug, err)
	}

	pgclient := flypg.New(pgApp.Name, dialer)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret.Name == *input.VariableName {
			return fmt.Errorf("consumer app %q already contains a secret named %s", appName, *input.VariableName)
		}
	}

	// Check to see if database exists
	dbExists, err := pgclient.DatabaseExists(ctx, *input.DatabaseName)
	if err != nil {
		return err
	}
	if dbExists {
		confirm := false
		msg := fmt.Sprintf("Database %q already exists. Continue with the attachment process?", *input.DatabaseName)
		confirm, err := prompt.Confirm(ctx, msg)
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Check to see if user exists
	usrExists, err := pgclient.UserExists(ctx, *input.DatabaseUser)
	if err != nil {
		return err
	}
	if usrExists {
		return fmt.Errorf("database user %q already exists. Please specify a new database user via --database-user", *input.DatabaseUser)
	}

	// Create attachment
	_, err = client.AttachPostgresCluster(ctx, input)
	if err != nil {
		return err
	}

	// Create database if it doesn't already exist
	if !dbExists {
		err := pgclient.CreateDatabase(ctx, *input.DatabaseName)
		if err != nil {
			if flypg.ErrorStatus(err) >= 500 {
				return err
			}
			return fmt.Errorf("error running database-create: %w", err)
		}
	}

	// Create user
	pwd, err := helpers.RandString(15)
	if err != nil {
		return err
	}

	err = pgclient.CreateUser(ctx, *input.DatabaseUser, pwd, true)
	if err != nil {
		return fmt.Errorf("failed executing create-user: %w", err)
	}

	connectionString := fmt.Sprintf("postgres://%s:%s@top2.nearest.of.%s.internal:5432/%s", *input.DatabaseUser, pwd, pgAppName, *input.DatabaseName)
	s := map[string]string{}
	s[*input.VariableName] = connectionString

	// TODO - We need to consider the possibility that the consumer app is another Machine.
	_, err = client.SetSecrets(ctx, appName, s)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is now attached to %s\n", pgAppName, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, *input.VariableName, connectionString)

	return nil
}

func hasRequiredVersionOnNomad(app *api.App, cluster, standalone string) error {
	// Validate image version to ensure it's compatible with this feature.
	if app.ImageDetails.Version == "" || app.ImageDetails.Version == "unknown" {
		return fmt.Errorf("command is not compatible with this image")
	}

	imageVersionStr := app.ImageDetails.Version[1:]
	imageVersion, err := version.NewVersion(imageVersionStr)
	if err != nil {
		return err
	}

	// Specify compatible versions per repo.
	requiredVersion := &version.Version{}
	if app.ImageDetails.Repository == "flyio/postgres-standalone" {
		requiredVersion, err = version.NewVersion(standalone)
		if err != nil {
			return err
		}
	}
	if app.ImageDetails.Repository == "flyio/postgres" {
		requiredVersion, err = version.NewVersion(cluster)
		if err != nil {
			return err
		}
	}

	if requiredVersion == nil {
		return fmt.Errorf("unable to resolve image version")
	}

	if imageVersion.LessThan(requiredVersion) {
		return fmt.Errorf(
			"image version is not compatible. (Current: %s, Required: >= %s)\n"+
				"Please run 'flyctl image show' and update to the latest available version",
			imageVersion, requiredVersion.String())
	}

	return nil
}

func hasRequiredVersionOnMachines() error {
	return nil
}
