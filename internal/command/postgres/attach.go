package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

type AttachParams struct {
	DbName       string
	AppName      string
	PgAppName    string
	DbUser       string
	VariableName string
	SuperUser    bool
	Force        bool
}

func newAttach() *cobra.Command {
	const (
		short = "Attach a postgres cluster to an app"
		long  = short + "\n"
		usage = "attach <POSTGRES APP>"
	)

	cmd := command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
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
		flag.Bool{
			Name:        "superuser",
			Default:     true,
			Description: "Grants attached user superuser privileges",
		},
		flag.Yes(),
	)

	return cmd
}

func runAttach(ctx context.Context) error {
	var (
		pgAppName = flag.FirstArg(ctx)
		appName   = appconfig.NameFromContext(ctx)
		client    = flyutil.ClientFromContext(ctx)
	)

	pgApp, err := client.GetAppCompact(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	if !pgApp.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", pgAppName)
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	// Build context around the postgres app
	ctx, err = apps.BuildContext(ctx, pgApp)
	if err != nil {
		return err
	}

	params := AttachParams{
		AppName:      app.Name,
		PgAppName:    pgApp.Name,
		DbName:       flag.GetString(ctx, "database-name"),
		DbUser:       flag.GetString(ctx, "database-user"),
		VariableName: flag.GetString(ctx, "variable-name"),
		Force:        flag.GetBool(ctx, "yes"),
		SuperUser:    flag.GetBool(ctx, "superuser"),
	}

	ips, err := client.GetIPAddresses(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving IP addresses for postgres app %s: %w", pgAppName, err)
	}

	var flycast *string

	for _, ip := range ips {
		if ip.Type == "private_v6" {
			flycast = &ip.Address
		}
	}

	return machineAttachCluster(ctx, params, flycast)
}

// AttachCluster is mean't to be called from an external package.
func AttachCluster(ctx context.Context, params AttachParams) error {
	var (
		client = flyutil.ClientFromContext(ctx)

		pgAppName = params.PgAppName
		appName   = params.AppName
	)

	pgApp, err := client.GetAppCompact(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	if !pgApp.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", pgAppName)
	}

	ctx, err = apps.BuildContext(ctx, pgApp)
	if err != nil {
		return err
	}

	// Verify that the target app exists.
	_, err = client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	ips, err := client.GetIPAddresses(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving IP addresses for postgres app %s: %w", pgAppName, err)
	}

	var flycast *string

	for _, ip := range ips {
		if ip.Type == "private_v6" {
			flycast = &ip.Address
		}
	}
	return machineAttachCluster(ctx, params, flycast)
}

func machineAttachCluster(ctx context.Context, params AttachParams, flycast *string) error {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresStandaloneVersion = "0.0.7"
		MinPostgresFlexVersion       = "0.0.3"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no active machines found")
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return runAttachCluster(ctx, leader.PrivateIP, params, flycast)
}

func runAttachCluster(ctx context.Context, leaderIP string, params AttachParams, flycast *string) error {
	var (
		client = flyutil.ClientFromContext(ctx)
		dialer = agent.DialerFromContext(ctx)
		io     = iostreams.FromContext(ctx)

		appName   = params.AppName
		pgAppName = params.PgAppName
		dbName    = params.DbName
		dbUser    = params.DbUser
		varName   = params.VariableName
		force     = params.Force
		superuser = params.SuperUser
	)

	if dbName == "" {
		dbName = appName
	}

	if dbUser == "" {
		dbUser = appName
	}

	dbUser = strings.ToLower(strings.ReplaceAll(dbUser, "-", "_"))

	if varName == "" {
		varName = "DATABASE_URL"
	}

	dbName = strings.ToLower(strings.ReplaceAll(dbName, "-", "_"))

	input := fly.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: pgAppName,
		ManualEntry:          true,
		DatabaseName:         fly.StringPointer(dbName),
		DatabaseUser:         fly.StringPointer(dbUser),
		VariableName:         fly.StringPointer(varName),
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	fmt.Fprintln(io.Out, "Checking for existing attachments")

	secrets, err := client.GetAppSecrets(ctx, input.AppID)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret.Name == *input.VariableName {
			return fmt.Errorf("consumer app %q already contains a secret named %s", input.AppID, *input.VariableName)
		}
	}

	// Check to see if database exists
	dbExists, err := pgclient.DatabaseExists(ctx, *input.DatabaseName)
	if err != nil {
		return err
	}
	if dbExists && !force {
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

	fmt.Fprintln(io.Out, "Registering attachment")

	// Create attachment
	_, err = client.AttachPostgresCluster(ctx, input)
	if err != nil {
		return err
	}

	// Create database if it doesn't already exist
	if !dbExists {
		fmt.Fprintln(io.Out, "Creating database")

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

	fmt.Fprintln(io.Out, "Creating user")

	err = pgclient.CreateUser(ctx, *input.DatabaseUser, pwd, superuser)
	if err != nil {
		return fmt.Errorf("failed executing create-user: %w", err)
	}

	connectionString := fmt.Sprintf(
		"postgres://%s:%s@top2.nearest.of.%s.internal:5432/%s?sslmode=disable",
		*input.DatabaseUser, pwd, input.PostgresClusterAppID, *input.DatabaseName,
	)
	if flycast != nil {
		connectionString = fmt.Sprintf(
			"postgres://%s:%s@%s.flycast:5432/%s?sslmode=disable",
			*input.DatabaseUser, pwd, input.PostgresClusterAppID, *input.DatabaseName,
		)
	}
	s := map[string]string{}
	s[*input.VariableName] = connectionString

	_, err = client.SetSecrets(ctx, input.AppID, s)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is now attached to %s\n", input.PostgresClusterAppID, input.AppID)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", input.AppID, *input.VariableName, connectionString)

	return nil
}
