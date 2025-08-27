package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/iostreams"
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
		newDeploy(),
		newKeys(),
	)

	return secrets
}

// getFlapsClient builds and returns a flaps client for the App from the context.
// Note: it does not return a context with the flaps client set.
func getFlapsClient(ctx context.Context) (flapsutil.FlapsClient, error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	ctx, err = setFlapsClient(ctx, app)
	if err != nil {
		return nil, err
	}

	return flapsutil.ClientFromContext(ctx), nil
}

// setFlapsClient builds a flaps client for app and stores it in a new context.
// On error the old context is returned along with the error.
func setFlapsClient(ctx context.Context, app *fly.AppCompact) (context.Context, error) {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return ctx, flyerr.GenericErr{
			Err: fmt.Sprintf("could not create flaps client: %v", err),
		}
	}

	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	return ctx, nil
}

// DeploySecrets deploys machines with the new secret if this step is not to be skipped.
// Note: setFlapsClient should be called before calling this function.
func DeploySecrets(ctx context.Context, app *fly.AppCompact, stage bool, detach bool) error {
	out := iostreams.FromContext(ctx).Out

	if stage {
		fmt.Fprint(out, "Secrets have been staged, but not set on VMs. Deploy or update machines in this app for the secrets to take effect.\n")
		return nil
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	if flapsClient == nil {
		return fmt.Errorf("DeploySecrets requires setFlapsClient to be called")
	}

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
		sentry.CaptureExceptionWithAppInfo(ctx, err, "secrets", app)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "secrets", app)
	}
	return err
}
