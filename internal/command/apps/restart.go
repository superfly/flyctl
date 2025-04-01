package apps

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
)

func newRestart() *cobra.Command {
	const (
		long  = `Restart an application. Perform a rolling restart against all running Machines.`
		short = "Restart an application."
		usage = "restart <app name>"
	)

	cmd := command.New(usage, short, long, runRestart,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	// Note -
	flag.Add(cmd,
		flag.Bool{
			Name:        "force-stop",
			Description: "Performs a force stop against the target Machine",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Restarts app without waiting for health checks",
			Default:     false,
		},
	)

	cmd.ValidArgsFunction = completion.Adapt(completion.CompleteApps)

	return cmd
}

func runRestart(ctx context.Context) error {
	var (
		appName = flag.FirstArg(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	if appName == "" {
		appName = appconfig.NameFromContext(ctx)
		if appName == "" {
			return errors.New("no app name was provided, and none is available from the environment or fly.toml")
		}
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.IsPostgresApp() {
		return fmt.Errorf("postgres apps should use `fly pg restart` instead")
	}

	ctx, err = BuildContext(ctx, app)
	if err != nil {
		return err
	}
	return runMachinesRestart(ctx, app)
}

func runMachinesRestart(ctx context.Context, app *fly.AppCompact) error {
	input := &fly.RestartMachineInput{
		ForceStop:        flag.GetBool(ctx, "force-stop"),
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
	}

	// Rolling restart against exclusively the machines managed by the Apps platform
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	machines, releaseFunc, err := machine.AcquireLeases(ctx, machines)
	defer releaseFunc()
	if err != nil {
		return err
	}

	for _, m := range machines {
		// Restart only if started
		// If you change this condition ensure standby machines aren't started when not running
		if m.State != fly.MachineStateStarted {
			continue
		}

		if err := machine.Restart(ctx, m, input, m.LeaseNonce); err != nil {
			return err
		}
	}

	return nil
}
