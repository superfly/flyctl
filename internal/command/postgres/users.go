package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
		flag.JSONOutput(),
	)

	return cmd
}

func runListUsers(ctx context.Context) error {
	var (
		client  = flyutil.ClientFromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
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
	return runMachineListUsers(ctx, app)
}

func runMachineListUsers(ctx context.Context, app *fly.AppCompact) (err error) {
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

	if err := hasRequiredVersionOnMachines(app.Name, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return renderUsers(ctx, leader.PrivateIP)
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

	rows := make([][]string, 0, len(users))

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
