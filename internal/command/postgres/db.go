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

func newDb() *cobra.Command {
	const (
		short = "Manage databases in a cluster"
		long  = short + "\n"
	)

	cmd := command.New("db", short, long, nil)

	cmd.AddCommand(
		newListDbs(),
	)

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

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runListDbs(ctx context.Context) error {
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
		return runMachineListDbs(ctx, app)
	case "nomad":
		return runNomadListDbs(ctx, app)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func runMachineListDbs(ctx context.Context, app *api.AppCompact) error {
	var (
		MinPostgresHaVersion = "0.0.19"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no 6pn ips founds for %s app", app.Name)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return listDBs(ctx, leader.PrivateIP)
}

func runNomadListDbs(ctx context.Context, app *api.AppCompact) error {
	// Minimum image version requirements
	var (
		MinPostgresHaVersion = "0.0.19"
		client               = client.FromContext(ctx).API()
	)

	if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
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

	return listDBs(ctx, leaderIP)

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

	rows := make([][]string, len(databases))
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
