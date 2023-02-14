package postgres

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
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
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
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
		if err := doAddFlycast(ctx); err != nil {
			return err
		}

		fmt.Fprintln(io.Out, "Flycast added!")
	case "nomad":
		return fmt.Errorf("not supported on nomad")
	default:
		return fmt.Errorf("unknown platform version")
	}
	return nil
}

func doAddFlycast(ctx context.Context) error {
	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	bouncerPort := 5432
	pgPort := 5433
	for _, machine := range machines {
		for _, service := range machine.Config.Services {
			if service.InternalPort == 5432 || service.InternalPort == 5433 {
				return fmt.Errorf("failed to enable flycast for pg machine %s because a service already exists on the postgres port(s)", machine.ID)
			}
		}

		confirm := false
		prompt := &survey.Confirm{
			Message: "This will overwrite existing services you have manually added. Continue?",
			Default: true,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}

		if !confirm {
			return nil
		}

		conf := machine.Config
		conf.Services =
			[]api.MachineService{
				{
					Protocol:     "tcp",
					InternalPort: bouncerPort,
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
					InternalPort: pgPort,
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

		err = mach.Update(ctx, machine, &api.LaunchMachineInput{
			Config: conf,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
