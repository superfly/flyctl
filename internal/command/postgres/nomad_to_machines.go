package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
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
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
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

	fmt.Fprintln(io.Out, "Preparing migration by scaling to zero. This may take a minute...")
	retryMax := 3
	count := 0
	// Nomad can be slow to spin down allocations so we may have to retry
	// a few times.
	for count <= retryMax {
		if err := prepare(ctx, app); err != nil {
			if strings.Contains(err.Error(), "Timeout") {
				count++
				continue
			}
			return err
		}
		break
	}

	fmt.Fprintln(io.Out, "Preparation complete")
	fmt.Fprintln(io.Out, "Starting migration")
	_, err = client.MigrateNomadToMachines(ctx, api.NomadToMachinesMigrationInput{AppID: app.Name})
	if err != nil {
		return err
	}
	fmt.Fprintln(io.Out, "Monitoring provisioned Machines")

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return err
	}

	if err := watch.MachinesChecks(ctx, machines); err != nil {
		return fmt.Errorf("failed to wait for health checks to pass: %w", err)
	}

	fmt.Fprintln(io.Out, "Migration complete!")

	return nil
}

func prepare(ctx context.Context, app *api.AppCompact) error {
	var (
		client = client.FromContext(ctx).API()
	)
	_, err := client.MigrateNomadToMachinesPrep(ctx, api.NomadToMachinesMigrationPrepInput{AppID: app.Name})

	return err
}
