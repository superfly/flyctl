package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
)

func newConnect() *cobra.Command {
	const (
		short = "Connect to the Postgres console"
		long  = short + "\n"

		usage = "connect"
	)

	cmd := command.New(usage, short, long, runConnect,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The name of the database you would like to connect to",
			Default:     "postgres",
		},
		flag.String{
			Name:        "user",
			Shorthand:   "u",
			Description: "The postgres user to connect with",
			Default:     "postgres",
		},
		flag.String{
			Name:        "password",
			Shorthand:   "p",
			Description: "The postgres user password",
		},
	)

	return cmd
}

func runConnect(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	switch app.PlatformVersion {
	case "machines":
		return runMachineConnect(ctx, app)
	case "nomad":
		return runNomadConnect(ctx, app)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func runMachineConnect(ctx context.Context, app *api.AppCompact) error {
	var (
		MinPostgresStandaloneVersion = "0.0.4"
		MinPostgresHaVersion         = "0.0.9"

		database = flag.GetString(ctx, "database")
		user     = flag.GetString(ctx, "user")
		password = flag.GetString(ctx, "password")
	)

	flapsClient := flaps.FromContext(ctx)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}
	return ssh.SSHConnect(&ssh.SSHParams{
		Ctx:    ctx,
		Org:    app.Organization,
		Dialer: agent.DialerFromContext(ctx),
		App:    app.Name,
		Cmd:    fmt.Sprintf("connect %s %s %s", database, user, password),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, leader.PrivateIP)
}

func runNomadConnect(ctx context.Context, app *api.AppCompact) error {
	var (
		client = client.FromContext(ctx).API()

		MinPostgresStandaloneVersion = "0.0.4"
		MinPostgresHaVersion         = "0.0.9"

		database = flag.GetString(ctx, "database")
		user     = flag.GetString(ctx, "user")
		password = flag.GetString(ctx, "password")
	)

	if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to establish agent: %w", err)
	}

	pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
	}
	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", app.Name)
	}
	leaderIP, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	return ssh.SSHConnect(&ssh.SSHParams{
		Ctx:    ctx,
		Org:    app.Organization,
		Dialer: agent.DialerFromContext(ctx),
		App:    app.Name,
		Cmd:    fmt.Sprintf("connect %s %s %s", database, user, password),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, leaderIP)

}
