package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
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
	Superuser    bool
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
		appName   = app.NameFromContext(ctx)
		client    = client.FromContext(ctx).API()
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
		Superuser:    true, // Default for PG's running Stolon
	}

	pgAppFull, err := client.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	var flycast *string

	for _, ip := range pgAppFull.IPAddresses.Nodes {
		if ip.Type == "private_v6" {
			flycast = &ip.Address
		}
	}

	switch pgApp.PlatformVersion {
	case "machines":
		return machineAttachCluster(ctx, params, flycast)
	case "nomad":
		return nomadAttachCluster(ctx, pgApp, params)
	default:
		return fmt.Errorf("platform is not supported")
	}
}

// AttachCluster is mean't to be called from an external package.
func AttachCluster(ctx context.Context, params AttachParams) error {
	var (
		client = client.FromContext(ctx).API()

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
	_, err = client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	pgAppFull, err := client.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	var flycast *string

	for _, ip := range pgAppFull.IPAddresses.Nodes {
		if ip.Type == "private_v6" {
			flycast = &ip.Address
		}
	}

	switch pgApp.PlatformVersion {
	case "machines":
		return machineAttachCluster(ctx, params, flycast)
	case "nomad":
		return nomadAttachCluster(ctx, pgApp, params)
	default:
		return fmt.Errorf("platform is not supported")
	}
}

func nomadAttachCluster(ctx context.Context, pgApp *api.AppCompact, params AttachParams) error {
	var (
		MinPostgresHaVersion = "0.0.19"
		client               = client.FromContext(ctx).API()
	)

	if err := hasRequiredVersionOnNomad(pgApp, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	pgInstances, err := agentclient.Instances(ctx, pgApp.Organization.Slug, pgApp.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", pgApp.Name, err)
	}

	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", pgApp.Name)
	}

	leaderIP, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	return runAttachCluster(ctx, leaderIP, params, nil)
}

func machineAttachCluster(ctx context.Context, params AttachParams, flycast *string) error {
	//Minimum image version requirements
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
		client = client.FromContext(ctx).API()
		dialer = agent.DialerFromContext(ctx)
		io     = iostreams.FromContext(ctx)

		appName   = params.AppName
		pgAppName = params.PgAppName
		dbName    = params.DbName
		dbUser    = params.DbUser
		varName   = params.VariableName
		force     = params.Force
		superuser = params.Superuser
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

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: pgAppName,
		ManualEntry:          true,
		DatabaseName:         api.StringPointer(dbName),
		DatabaseUser:         api.StringPointer(dbUser),
		VariableName:         api.StringPointer(varName),
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
			"postgres://%s:%s@[%s]:5432/%s?sslmode=disable",
			*input.DatabaseUser, pwd, *flycast, *input.DatabaseName,
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
