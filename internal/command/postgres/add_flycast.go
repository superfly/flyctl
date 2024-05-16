package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
)

func newAddFlycast() *cobra.Command {
	const (
		short = "Switch from DNS to flycast based pg connections"
		long  = short + "\n"

		usage = "add_flycast"
	)

	cmd := command.New(usage, short, long, runAddFlycast,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Hidden = true

	return cmd
}

func runAddFlycast(ctx context.Context) error {
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

	if err := doAddFlycast(ctx); err != nil {
		return err
	}

	return nil
}

func doAddFlycast(ctx context.Context) error {
	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	var bouncerPort int = 5432
	var pgPort int = 5433
	for _, machine := range machines {
		for _, service := range machine.Config.Services {
			if service.InternalPort == 5432 || service.InternalPort == 5433 {
				return fmt.Errorf("failed to enable flycast for pg machine %s because a service already exists on the postgres port(s)", machine.ID)
			}
		}

		message := "This will overwrite existing services you have manually added. Continue?"

		confirm, err := prompt.Confirm(ctx, message)
		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}

		conf := machine.Config
		conf.Services = []fly.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: bouncerPort,
				Ports: []fly.MachinePort{
					{
						Port: &bouncerPort,
						Handlers: []string{
							"pg_tls",
						},
						ForceHTTPS: false,
					},
				},
				Concurrency: nil,
			},
			{
				Protocol:     "tcp",
				InternalPort: pgPort,
				Ports: []fly.MachinePort{
					{
						Port: &pgPort,
						Handlers: []string{
							"pg_tls",
						},
						ForceHTTPS: false,
					},
				},
				Concurrency: nil,
			},
		}

		err = mach.Update(ctx, machine, &fly.LaunchMachineInput{
			Config: conf,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
