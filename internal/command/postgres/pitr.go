package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
		client = flyutil.ClientFromContext(ctx)
	)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return false, err
	}

	for _, secret := range secrets {
		if secret.Name == flypg.BarmanSecretName {
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

	// flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
	// AppName: appName,
	// })
	// if err != nil {
	// 	return fmt.Errorf("list of machines could not be retrieved: %w", err)
	// }

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
		PitrEnabled:  true,
	}

	err = flypg.CreateTigrisBucket(ctx, *pgInput)
	if err != nil {
		return err
	}

	secrets := map[string]string{
		flypg.BarmanSecretName: pgInput.BarmanSecret,
	}

	if _, err := client.SetSecrets(ctx, appName, secrets); err != nil {
		return err
	}
	// TODO: Update deployment with new secrets
	return nil
}
