package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newAttach() (cmd *cobra.Command) {
	const (
		long = `Attach Postgres to an existing App
`
		short = "Attach Postgres to an existing App"
		usage = "attach [POSTGRES APP]"
	)

	cmd = command.New(usage, short, long, runAttach,
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

	return
}

func runAttach(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	pgAppName := flag.FirstArg(ctx)

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

	client := client.FromContext(ctx).API()

	pgApp, err := client.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	_, err = client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgCmd, err := newPostgresCmd(ctx, pgApp)
	if err != nil {
		return err
	}

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret.Name == *input.VariableName {
			return fmt.Errorf("consumer app %q already contains an environment variable named %s", appName, *input.VariableName)
		}
	}
	// Check to see if database exists
	dbExists, err := pgCmd.DbExists(*input.DatabaseName)
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
	usrExists, err := pgCmd.userExists(*input.DatabaseUser)
	if err != nil {
		return err
	}
	if usrExists {
		return fmt.Errorf("Database user %q already exists. Please specify a new database user via --database-user", *input.DatabaseUser)
	}

	// Create attachment
	_, err = client.AttachPostgresCluster(ctx, input)
	if err != nil {
		return err
	}

	// Create database if it doesn't already exist
	if !dbExists {
		dbResp, err := pgCmd.createDatabase(*input.DatabaseName)
		if err != nil {
			return err
		}
		if dbResp.Error != "" {
			return errors.Wrap(fmt.Errorf(dbResp.Error), "executing database-create")
		}
	}

	// Create user
	pwd, err := helpers.RandString(15)
	if err != nil {
		return err
	}

	usrResp, err := pgCmd.createUser(*input.DatabaseUser, pwd)
	if err != nil {
		return err
	}
	if usrResp.Error != "" {
		return errors.Wrap(fmt.Errorf(usrResp.Error), "executing create-user")
	}

	// Grant access
	gaResp, err := pgCmd.grantAccess(*input.DatabaseName, *input.DatabaseUser)
	if err != nil {
		return err
	}
	if gaResp.Error != "" {
		return errors.Wrap(fmt.Errorf(usrResp.Error), "executing grant-access")
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
