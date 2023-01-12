package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newNomadToMachines() *cobra.Command {
	const (
		short = "Migrate Nomad cluster to Machines"
		long  = short + "\n"

		usage = "migrate_to_machines"
	)

	cmd := command.New(usage, short, long, runNomadToMachinesMigration,
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

type Migrations struct {
	migrations []Migration
}

type Migration struct {
	Healthy bool
	Region  string
	Volume  *api.Volume
}

func runNomadToMachinesMigration(ctx context.Context) error {
	var (
		client = client.FromContext(ctx).API()
		// io     = iostreams.FromContext(ctx)
		// cfg    = config.FromContext(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.PostgresAppRole.Name != "postgres_cluster" {
		return fmt.Errorf("app %s is not a Postgres app", app.Name)
	}

	if app.PlatformVersion != "nomad" {
		return fmt.Errorf("this app has already been migrated")
	}

	input := api.NomadToMachinesMigrationInput{
		AppID: app.Name,
	}

	_, err = client.MigrateNomadToMachines(ctx, input)
	if err != nil {
		return err
	}

	return nil
}
