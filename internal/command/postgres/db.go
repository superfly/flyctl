package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
	// Minimum image version requirements
	var (
		MinPostgresHaVersion = "0.0.19"
		appName              = app.NameFromContext(ctx)
		client               = client.FromContext(ctx).API()
		cfg                  = config.FromContext(ctx)
		io                   = iostreams.FromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("error getting app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("%s is not a postgres app", appName)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	switch app.PlatformVersion {
	case "nomad":
		if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
	case "machines":
		flapsClient, err := flaps.New(ctx, app)
		if err != nil {
			return fmt.Errorf("list of machines could not be retrieved: %w", err)
		}

		members, err := flapsClient.List(ctx, "started")
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}
		if err := hasRequiredVersionOnMachines(members, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported platform %s", app.PlatformVersion)
	}

	pgclient := flypg.New(appName, dialer)

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
