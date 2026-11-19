package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
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
		short = "Attach a Fly Postgres cluster to an app"
		long  = `Attach a Fly Postgres cluster to an app. Pass the Postgres app's name as
the argument; the consuming app comes from --app or fly.toml.

On the cluster, this creates a database and a user, both named after the
consuming app with hyphens turned into underscores unless --database-name
or --database-user say otherwise. The user is a superuser unless you pass
--superuser=false. The connection string is then set as a secret on the
consuming app (DATABASE_URL unless --variable-name says otherwise).

The command fails if the app already has that secret or the user already
exists. If the database already exists, you're asked whether to use it
anyway; pass --yes to skip the question, which is required without a
terminal. Undo it with ` + "`fly postgres detach`" + `.`
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
		pgAppName   = flag.FirstArg(ctx)
		appName     = appconfig.NameFromContext(ctx)
		flapsClient = flapsutil.ClientFromContext(ctx)
	)

	pgApp, err := flapsClient.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	if !flapsutil.IsPostgresApp(pgApp) {
		return fmt.Errorf("app %s is not a postgres app", pgAppName)
	}

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	// Build context around the postgres app
	ctx, err = apps.BuildContextForApp(ctx, pgApp)
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

	flycast, ips, err := flycastAddress(ctx, pgAppName)
	if err != nil {
		return err
	}
	warnIfUnreachable(ctx, pgApp, app, ips)

	return machineAttachCluster(ctx, params, flycast)
}

// AttachCluster is mean't to be called from an external package.
func AttachCluster(ctx context.Context, params AttachParams) error {
	var (
		flapsClient = flapsutil.ClientFromContext(ctx)

		pgAppName = params.PgAppName
		appName   = params.AppName
	)

	pgApp, err := flapsClient.GetApp(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("failed retrieving postgres app %s: %w", pgAppName, err)
	}

	if !flapsutil.IsPostgresApp(pgApp) {
		return fmt.Errorf("app %s is not a postgres app", pgAppName)
	}

	ctx, err = apps.BuildContextForApp(ctx, pgApp)
	if err != nil {
		return err
	}

	// Verify that the target app exists.
	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	flycast, ips, err := flycastAddress(ctx, pgAppName)
	if err != nil {
		return err
	}
	warnIfUnreachable(ctx, pgApp, app, ips)

	return machineAttachCluster(ctx, params, flycast)
}

// warnIfUnreachable prints unreachableWarning, if any, to stderr. It only
// warns: the user may be about to fix the network, and attach still works for
// apps that can reach the cluster some other way.
func warnIfUnreachable(ctx context.Context, pgApp, app *flaps.App, ips []flaps.IPAssignment) {
	if msg := unreachableWarning(pgApp, app, ips); msg != "" {
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.ErrOut, "%s %s\n", io.ColorScheme().WarningIcon(), msg)
	}
}

// unreachableWarning explains why app can't reach pgApp's database when they
// are on different private networks, given pgApp's IP assignments, and
// returns "" when app can reach it or reachability can't be told.
func unreachableWarning(pgApp, app *flaps.App, ips []flaps.IPAssignment) string {
	pgNetwork, appNetwork := flapsutil.NetworkName(pgApp), flapsutil.NetworkName(app)
	if pgNetwork == appNetwork {
		return ""
	}

	hasFlycast := false
	for _, ip := range ips {
		if !ip.IsFlycast() {
			continue
		}
		hasFlycast = true

		// Without a network on the assignment, it may be on app's network.
		if ip.Network == nil || flapsutil.NormalizeNetworkName(ip.Network.Name) == appNetwork {
			return ""
		}
	}

	mismatch := fmt.Sprintf("%s is on network %s and %s is on network %s, so %s won't be able to reach the database.",
		app.Name, displayNetwork(appNetwork), pgApp.Name, displayNetwork(pgNetwork), app.Name)

	// Without a Flycast address the connection string uses .internal, which
	// never crosses networks, so a new address wouldn't help.
	if !hasFlycast {
		return mismatch + " Its connection string uses .internal, which only works within one network, so put both apps on the same network."
	}

	// The default network is selected by leaving --network off.
	allocate := fmt.Sprintf("fly ips allocate-v6 --private -a %s", pgApp.Name)
	if appNetwork != "" {
		allocate = fmt.Sprintf("fly ips allocate-v6 --private --network %s -a %s", appNetwork, pgApp.Name)
	}

	return fmt.Sprintf("%s To fix it, give %s a Flycast address on %s's network: %s", mismatch, pgApp.Name, app.Name, allocate)
}

// displayNetwork names a network as users and the CLI flags know it.
func displayNetwork(name string) string {
	if name == "" {
		return flapsutil.DefaultNetwork
	}

	return name
}

// flycastAddress returns the postgres app's Flycast (private IPv6) address, if
// it has one, along with all of its IP assignments.
func flycastAddress(ctx context.Context, pgAppName string) (*string, []flaps.IPAssignment, error) {
	res, err := flapsutil.ClientFromContext(ctx).GetIPAssignments(ctx, pgAppName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed retrieving IP addresses for postgres app %s: %w", pgAppName, err)
	}

	var flycast *string
	for _, ip := range res.IPs {
		if ip.IsFlycast() {
			flycast = &ip.IP
		}
	}

	return flycast, res.IPs, nil
}

func machineAttachCluster(ctx context.Context, params AttachParams, flycast *string) error {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresStandaloneVersion = "0.0.7"
		MinPostgresFlexVersion       = "0.0.3"
	)

	machines, err := mach.ListActive(ctx, params.PgAppName)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no active machines found")
	}

	if err := hasRequiredVersionOnMachines(params.PgAppName, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
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

	flapsClient := flapsutil.ClientFromContext(ctx)

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
		DatabaseName:         new(dbName),
		DatabaseUser:         new(dbUser),
		VariableName:         new(varName),
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	fmt.Fprintln(io.Out, "Checking for existing attachments")

	secrets, err := appsecrets.List(ctx, flapsClient, appName)
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
		msg := fmt.Sprintf("Database %q already exists. Continue with the attachment process?", *input.DatabaseName)
		switch confirm, err := prompt.Confirm(ctx, msg); {
		case err == nil:
			if !confirm {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError(fmt.Sprintf("database %q already exists; pass --yes to attach to it when not running interactively", *input.DatabaseName))
		default:
			return err
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

	err = appsecrets.Update(ctx, flapsClient, appName, s, nil)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is now attached to %s\n", input.PostgresClusterAppID, input.AppID)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", input.AppID, *input.VariableName, connectionString)

	return nil
}
