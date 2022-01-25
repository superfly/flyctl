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
	"github.com/superfly/flyctl/pkg/agent"
)

func newAttach() (cmd *cobra.Command) {
	const (
		long = `Attach Postgres to an existing App
`
		short = "Attach Postgres to an existing App"
		usage = "attach"
	)

	cmd = command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	// attachStrngs := docstrings.Get("postgres.attach")
	// attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, client, requireSession, requireAppName)
	// attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to attach to the app"})
	// attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name", Description: "database to use, defaults to a new database with the same name as the app"})
	// attachCmd.AddStringFlag(StringFlagOpts{Name: "database-user", Description: "the database user to create, defaults to creating a user with the same name as the consuming app"})
	// attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name", Description: "the env variable name that will be added to the app. Defaults to DATABASE_URL"})

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "consumer-app",
			Description: "The name of the consuming app.",
		},
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
	providerAppName := app.NameFromContext(ctx)

	consumerAppName := flag.GetString(ctx, "consumer-app")
	if consumerAppName == "" {
		return fmt.Errorf("consumer-app is required")
	}

	dbName := flag.GetString(ctx, "database-name")
	if dbName == "" {
		dbName = consumerAppName
	}
	dbName = strings.ToLower(strings.ReplaceAll(dbName, "-", "_"))

	dbUser := flag.GetString(ctx, "database-user")
	if dbUser == "" {
		dbUser = consumerAppName
	}
	dbUser = strings.ToLower(strings.ReplaceAll(dbUser, "-", "_"))

	varName := flag.GetString(ctx, "variable-name")
	if varName == "" {
		varName = "DATABASE_URL"
	}

	input := api.AttachPostgresClusterInput{
		AppID:                consumerAppName,
		PostgresClusterAppID: providerAppName,
		ManualEntry:          true,
		DatabaseName:         api.StringPointer(dbName),
		DatabaseUser:         api.StringPointer(dbUser),
		VariableName:         api.StringPointer(varName),
	}

	client := client.FromContext(ctx).API()

	providerApp, err := client.GetApp(ctx, providerAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	_, err = client.GetApp(ctx, consumerAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, providerApp.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh cant build tunnel for %s: %s\n", providerApp.Organization.Slug, err)
	}

	pgCmd := newPostgresCmd(ctx, providerApp, dialer)

	secrets, err := client.GetAppSecrets(ctx, consumerAppName)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret.Name == *input.VariableName {
			return fmt.Errorf("consumer app %q already contains a secret named %s\n", consumerAppName, *input.VariableName)
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

	connectionString := fmt.Sprintf("postgres://%s:%s@top2.nearest.of.%s.internal:5432/%s", *input.DatabaseUser, pwd, providerAppName, *input.DatabaseName)
	s := map[string]string{}
	s[*input.VariableName] = connectionString

	_, err = client.SetSecrets(ctx, consumerAppName, s)
	if err != nil {
		return err
	}

	fmt.Printf("\nPostgres cluster %s is now attached to %s\n", providerAppName, consumerAppName)
	fmt.Printf("The following secret was added to %s:\n  %s=%s\n", consumerAppName, *input.VariableName, connectionString)

	return nil
}
