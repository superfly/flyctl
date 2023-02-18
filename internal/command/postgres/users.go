package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newUsers() *cobra.Command {
	const (
		short = "Manage users in a postgres cluster"
		long  = short + "\n"

		usage = "users"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newListUsers(),
	)

	return cmd
}

func newListUsers() *cobra.Command {
	const (
		short = "List users"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runListUsers,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runListUsers(ctx context.Context) error {
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
		return runMachineListUsers(ctx, app)
	case "nomad":
		return runNomadListUsers(ctx, app)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func runMachineListUsers(ctx context.Context, app *api.AppCompact) (err error) {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return renderUsers(ctx, leader.PrivateIP)
}

func runNomadListUsers(ctx context.Context, app *api.AppCompact) (err error) {
	var (
		MinPostgresHaVersion = "0.0.19"
		client               = client.FromContext(ctx).API()
	)

	if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
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

	return renderUsers(ctx, leaderIP)
}

func renderUsers(ctx context.Context, leaderIP string) error {
	var (
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)
	)

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	users, err := pgclient.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("error fetching users: %w", err)
	}

	if len(users) == 0 {
		fmt.Fprintf(io.Out, "No users found\n")
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, users)
	}

	rows := make([][]string, len(users))

	for _, user := range users {
		var databases string

		for i, database := range user.Databases {
			databases += database

			if i < len(user.Databases)-1 {
				databases += ", "
			}
		}

		superuser := "no"
		if user.Superuser {
			superuser = "yes"
		}

		rows = append(rows, []string{
			user.Username,
			superuser,
			databases,
		})
	}

	return render.Table(io.Out, "", rows, "Name", "Superuser", "Databases")
}
