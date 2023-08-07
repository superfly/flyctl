package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.Detach(),
	flag.Bool{
		Name:        "stage",
		Description: "Set secrets but skip deployment for machine apps",
	},
}

func New() *cobra.Command {
	const (
		long = `Secrets are provided to applications at runtime as ENV variables. Names are
		case sensitive and stored as-is, so ensure names are appropriate for
		the application and vm environment.
		`

		short = "Manage application secrets with the set and unset commands."
	)

	secrets := command.New("secrets", short, long, nil)

	secrets.AddCommand(
		newList(),
		newSet(),
		newUnset(),
		newImport(),
	)

	return secrets
}

func deployForSecrets(ctx context.Context, app *api.AppCompact, release *api.Release, stage bool, detach bool) error {
	switch app.PlatformVersion {
	case appconfig.MachinesPlatform:
		return DeploySecrets(ctx, app, stage, detach)
	default:
		return v1deploySecrets(ctx, app, release, stage, detach)
	}
}

func v1deploySecrets(ctx context.Context, app *api.AppCompact, release *api.Release, stage bool, detach bool) error {
	out := iostreams.FromContext(ctx).Out
	if stage {
		return errors.New("--stage isn't available for Nomad apps")
	}

	if !app.Deployed {
		fmt.Fprintln(out, "Secrets are staged for the first deployment")
		return nil
	}

	fmt.Fprintf(out, "Release v%d created\n", release.Version)
	if flag.GetBool(ctx, "detach") {
		return nil
	}

	return watch.Deployment(ctx, app.Name, release.EvaluationID)
}

func DeploySecrets(ctx context.Context, app *api.AppCompact, stage bool, detach bool) error {
	out := iostreams.FromContext(ctx).Out

	if stage {
		fmt.Fprint(out, "Secrets have been staged, but not set on VMs. Deploy or update machines in this app for the secrets to take effect.\n")
		return nil
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// Due to https://github.com/superfly/web/issues/1397 we have to be extra careful
	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}
	if !app.Deployed && len(machines) == 0 {
		fmt.Fprintln(out, "Secrets are staged for the first deployment")
		return nil
	}

	// It would be confusing for setting secrets to deploy the current fly.toml file.
	// Instead, we always grab the currently deployed app config
	cfg, err := appconfig.FromRemoteApp(ctx, app.Name)
	if err != nil {
		return fmt.Errorf("error loading appv2 config: %w", err)
	}
	ctx = appconfig.WithConfig(ctx, cfg)

	md, err := deploy.NewMachineDeployment(ctx, deploy.MachineDeploymentArgs{
		AppCompact:       app,
		RestartOnly:      true,
		SkipHealthChecks: detach,
	})
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "secrets", app)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "secrets", app)
	}
	return err

}
