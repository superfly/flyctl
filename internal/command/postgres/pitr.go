package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
)

func newPitr() *cobra.Command {
	const (
		short = "Point-in-time recovery commands"
		long  = short + "\n"
	)

	cmd := command.New("pitr", short, long, nil)

	cmd.AddCommand(newPitrEnable())
	return cmd
}

func newPitrEnable() *cobra.Command {
	const (
		short = "Enable PITR on a Postgres cluster"
		long  = short + "\n"

		usage = "enable"
	)

	cmd := command.New(usage, short, long, runPitrEnable,
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

func isPitrEnabled(ctx context.Context, appName string) (bool, error) {
	var (
		client  = flyutil.ClientFromContext(ctx)
	)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return false, err
	}

	for _, secret := range secrets {
		if secret.Name == "BARMAN_ENABLED" {
			return true, nil
		}
	}

	return false, nil
}

func runPitrEnable(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	enabled, err := isPitrEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if enabled {
		return fmt.Errorf("PITR is already enabled.")
	}

	org, err := client.GetOrganizationByApp(ctx, appName)
	pgInput := &flypg.CreateClusterInput{
		AppName:      appName,
		Organization: org,
	}

	err = flypg.CreateTigrisBucket(ctx, *pgInput)
	if err != nil {
		return err
	}

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return err
	}

	for _, machine := range machines {
		machine, releaseLeaseFunc, err := mach.AcquireLease(ctx, machine)
		defer releaseLeaseFunc()
		if err != nil {
			return err
		}
		input := &fly.LaunchMachineInput{
			Name:   machine.Name,
			Region: machine.Region,
			Config: machine.Config,
		}
		input.Config.Env["BARMAN_ENABLED"] = pgInput.BarmanSecret
		if err := mach.Update(ctx, machine, input); err != nil {
			var timeoutErr mach.WaitTimeoutErr
			if errors.As(err, &timeoutErr) {
				return flyerr.GenericErr{
					Err:      timeoutErr.Error(),
					Descript: timeoutErr.Description(),
					Suggest:  "Try increasing the --wait-timeout",
				}
			}
			return err
		}
	}

	return nil
}
