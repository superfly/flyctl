package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
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
		long = `Manage an app's secrets. Secrets are encrypted values that your app's
Machines receive as environment variables at runtime. Names are case
sensitive and stored as-is.

Use ` + "`fly secrets set`" + ` and ` + "`fly secrets unset`" + ` to change secrets; by
default each change restarts the app's Machines so they pick it up. Add
` + "`--stage`" + ` to change secrets without restarting, then apply staged changes
later with ` + "`fly secrets deploy`" + ` or your next ` + "`fly deploy`" + `. Use
` + "`fly secrets list`" + ` to see names, digests and deployment status.`

		short = "Manage app secrets"
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
func DeploySecrets(ctx context.Context, appName string, args DeploymentArgs) error {
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
	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("error getting app: %w", err)
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, app.Name)
	if err != nil {
		return err
	}
	if !app.Deployed() && len(machines) == 0 {
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
