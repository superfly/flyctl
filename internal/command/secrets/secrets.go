package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
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
	flag.Bool{
		Name:        "dns-checks",
		Description: "Perform DNS checks during deployment",
		Default:     true,
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
		newSync(),
		newUnset(),
		newImport(),
		newDeploy(),
		newKeys(),
	)

	return secrets
}

type DeploymentArgs struct {
	Stage    bool
	Detach   bool
	CheckDNS bool
}

// DeploySecrets deploys machines with the new secret if this step is not to be skipped.
func DeploySecrets(ctx context.Context, appCompact *fly.AppCompact, args DeploymentArgs) error {
	out := iostreams.FromContext(ctx).Out

	if args.Stage {
		fmt.Fprint(out, "Secrets have been staged, but not set on VMs. Deploy or update machines in this app for the secrets to take effect.\n")
		return nil
	}

	// Due to https://github.com/superfly/web/issues/1397 we have to be extra careful
	flapsClient := flapsutil.ClientFromContext(ctx)
	if flapsClient == nil {
		return fmt.Errorf("flaps client missing from context")
	}
	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, appCompact.Name)
	if err != nil {
		return err
	}
	if !appCompact.Deployed && len(machines) == 0 {
		fmt.Fprintln(out, "Secrets are staged for the first deployment")
		return nil
	}

	// It would be confusing for setting secrets to deploy the current fly.toml file.
	// Instead, we always grab the currently deployed app config
	cfg, err := appconfig.FromRemoteApp(ctx, appCompact.Name)
	if err != nil {
		return fmt.Errorf("error loading appv2 config: %w", err)
	}
	ctx = appconfig.WithConfig(ctx, cfg)

	// Re-fetch app from flaps so it's compatible with deploy.
	// This won't be needed once we're using the flaps.App everywhere.
	app, err := flapsClient.GetApp(ctx, appCompact.Name)
	if err != nil {
		return fmt.Errorf("error getting app: %w", err)
	}

	md, err := deploy.NewMachineDeployment(ctx, deploy.MachineDeploymentArgs{
		App:              app,
		RestartOnly:      true,
		SkipHealthChecks: args.Detach,
		SkipDNSChecks:    args.Detach || !args.CheckDNS,
	})
	if err != nil {
		sentry.CaptureExceptionWithFlapsAppInfo(ctx, err, "secrets", app)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithFlapsAppInfo(ctx, err, "secrets", app)
	}
	return err
}
