package apps

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newSetPlatformVersion() *cobra.Command {
	const (
		long = `The APPS SET-PLATFORM-VERSION command directly sets the platform version for an application.
Please use caution when using this command.`
		short = "Directly set the platform version"
		usage = "set-platform-version <nomad|detached|machines>"
	)

	cmd := command.New(usage, short, long, runSetPlatformVersion,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Hidden = true

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.ValidArgs = []string{"nomad", "detached", "machines"}

	return cmd
}

func runSetPlatformVersion(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.IsPostgresApp() {
		return fmt.Errorf("Postgres apps should use `fly migrate-to-v2` instead")
	}

	currentVersion := app.PlatformVersion
	desiredVersion := flag.FirstArg(ctx)

	if currentVersion == desiredVersion {
		fmt.Fprintf(io.Out, "App %s is already on platform version %s", appName, desiredVersion)
		return nil
	}

	if desiredVersion == "machines" {
		// Validate that there are no nomad allocs, and that autoscaling is disabled
		allocs, err := client.GetAllocations(ctx, appName, false)
		if err != nil {
			return err
		}
		if len(allocs) > 0 {
			return errors.New("cannot migrate to machines platform version with existing nomad allocations")
		}
	}
	if currentVersion == "nomad" {
		autoscaleConfig, err := client.AppAutoscalingConfig(ctx, appName)
		if err != nil {
			return err
		}
		if autoscaleConfig.Enabled {
			return fmt.Errorf("cannot migrate to %s platform version with autoscaling enabled", desiredVersion)
		}
	}
	if desiredVersion == "nomad" {
		// Validate that there are no machines
		flapsClient, err := flaps.New(ctx, app)
		if err != nil {
			return fmt.Errorf("list of machines could not be retrieved: %w", err)
		}

		machines, err := flapsClient.List(ctx, "")
		if err != nil {
			return fmt.Errorf("machines could not be retrieved")
		}

		if len(machines) > 0 {
			return errors.New("cannot migrate to nomad platform version with existing machines")
		}
	}

	switch confirmed, err := prompt.Confirmf(ctx, "Are you sure you want to change the platform version of %s from %s to %s?", appName, currentVersion, desiredVersion); {
	case err == nil:
		if !confirmed {
			return nil
		}
	case prompt.IsNonInteractive(err):
		return prompt.NonInteractiveError("flyctl apps set-platform-version can only be run interactively")
	default:
		return err
	}

	err = UpdateAppPlatformVersion(ctx, appName, desiredVersion)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "App %s is now on platform version %s\n", appName, desiredVersion)
	return nil
}

func UpdateAppPlatformVersion(ctx context.Context, appName string, platform string) error {
	_ = `# @genqlient
	mutation SelfServiceSetPlatformVersion($input:SetPlatformVersionInput!) {
		setPlatformVersion(input:$input) {
			app { id }
		}
	}
	`
	client := client.FromContext(ctx).API()
	input := gql.SetPlatformVersionInput{
		AppId:           appName,
		PlatformVersion: platform,
	}
	_, err := gql.SelfServiceSetPlatformVersion(ctx, client.GenqClient, input)
	if err != nil {
		return err
	}
	return nil
}
