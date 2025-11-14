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

func newDb() *cobra.Command {
	const (
		short = "Manage databases in a cluster"
		long  = short + "\n"
	)

	cmd := command.New("db", short, long, nil)

	cmd.AddCommand(
		newListDbs(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func newListDbs() *cobra.Command {
	const (
		short = "list databases"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runListDbs,
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

func runListDbs(ctx context.Context) error {
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
	return runMachineListDbs(ctx, app)
}

func runMachineListDbs(ctx context.Context, app *fly.AppCompact) error {
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no 6pn ips founds for %s app", app.Name)
	}

	if err := hasRequiredVersionOnMachines(app.Name, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return listDBs(ctx, leader.PrivateIP)
}

func listDBs(ctx context.Context, leaderIP string) error {
	var (
		dialer = agent.DialerFromContext(ctx)
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
	)

	pgclient := flypg.NewFromInstance(leaderIP, dialer)
	databases, err := pgclient.ListDatabases(ctx)
	if err != nil {
		return err
	}

	if len(databases) == 0 {
		fmt.Fprintf(io.Out, "No databases found\n")
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, databases)
	}

	rows := make([][]string, 0, len(databases))
	for _, db := range databases {
		var users string
		for index, name := range db.Users {
			users += name
			if index < len(db.Users)-1 {
				users += ", "
			}
		}
		rows = append(rows, []string{
			db.Name,
			users,
		})
	}

	return render.Table(io.Out, "", rows, "Name", "Users")
}
