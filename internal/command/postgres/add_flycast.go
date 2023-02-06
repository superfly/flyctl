package postgres

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
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
		flag.Yes(),
	)

	cmd.Hidden = true

	return cmd
}

func runAddFlycast(ctx context.Context) error {
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
		machines, err := mach.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}

		var bouncerPort int32 = 5432
		var pgPort int32 = 5433
		for _, machine := range machines {
			conf := machine.Config
			conf.Services =
				[]api.MachineService{
					{
						Protocol:     "tcp",
						InternalPort: 5432,
						Ports: []api.MachinePort{
							{
								Port: &bouncerPort,
								Handlers: []string{
									"pg_tls",
								},
								ForceHttps: false,
							},
						},
						Concurrency: nil,
					},
					{
						Protocol:     "tcp",
						InternalPort: 5433,
						Ports: []api.MachinePort{
							{
								Port: &pgPort,
								Handlers: []string{
									"pg_tls",
								},
								ForceHttps: false,
							},
						},
						Concurrency: nil,
					},
				}

			err := mach.Update(ctx, machine, &api.LaunchMachineInput{
				Config: conf,
			})
			if err != nil {
				return err
			}
		}

		fmt.Println("Flycast added!")
	case "nomad":
		return fmt.Errorf("not supported on nomad")
	default:
		return fmt.Errorf("unknown platform version")
	}
	return nil
}
