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
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach a postgres cluster to an app"
		long  = short + "\n"
		usage = "attach [POSTGRES APP]"
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
			Name:        "force",
			Default:     false,
			Description: "Force attach (bypass confirmation)",
		})

	return cmd
}

type AttachParams struct {
	DbName       string
	AppName      string
	PgAppName    string
	DbUser       string
	VariableName string
	Force        bool
}

func runAttach(ctx context.Context) error {

	params := AttachParams{
		AppName:      app.NameFromContext(ctx),
		DbName:       flag.GetString(ctx, "database-name"),
		PgAppName:    flag.FirstArg(ctx),
		DbUser:       flag.GetString(ctx, "database-user"),
		VariableName: flag.GetString(ctx, "variable-name"),
		Force:        flag.GetBool(ctx, "force"),
	}

	return AttachCluster(ctx, params)
}

func AttachCluster(ctx context.Context, params AttachParams) error {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion = "0.0.19"
		client               = client.FromContext(ctx).API()
		appName              = params.AppName
		pgAppName            = params.PgAppName
		dbName               = params.DbName
		dbUser               = params.DbUser
		varName              = params.VariableName
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

	pgApp, err := client.GetAppCompact(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, pgApp.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", pgApp.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	var leaderIp string
	switch pgApp.PlatformVersion {
	case "nomad":
		if err := hasRequiredVersionOnNomad(pgApp, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
		pgInstances, err := agentclient.Instances(ctx, pgApp.Organization.Slug, pgApp.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", pgAppName, err)
		}
		if len(pgInstances.Addresses) == 0 {
			return fmt.Errorf("no 6pn ips found for %s app", pgAppName)
		}
		leaderIp, err = leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
		if err != nil {
			return err
		}
	case "machines":
		flapsClient, err := flaps.New(ctx, pgApp)
		if err != nil {
			return fmt.Errorf("list of machines could not be retrieved: %w", err)
		}

		members, err := flapsClient.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}
		if err := hasRequiredVersionOnMachines(members, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
		leader, _ := machinesNodeRoles(ctx, members)
		leaderIp = leader.PrivateIP
	default:
	}

	pgclient := flypg.NewFromInstance(leaderIp, dialer)

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
	if dbExists && !params.Force {
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
