package apps

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newRestart() *cobra.Command {
	const (
		long  = `The APPS RESTART command will perform a rolling restart against all running VMs`
		short = "Restart an application"
		usage = "restart <APPNAME>"
	)

	cmd := command.New(usage, short, long, runRestart,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	// Note -
	flag.Add(cmd,
		flag.Bool{
			Name:        "force-stop",
			Description: "Performs a force stop against the target Machine. ( Machines only )",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Restarts app without waiting for health checks. ( Machines only )",
			Default:     false,
		},
	)

	return cmd
}

func runRestart(ctx context.Context) error {
	var (
		appName = flag.FirstArg(ctx)
		client  = client.FromContext(ctx).API()
	)

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

	if app.PlatformVersion == "machines" {
		return runMachinesRestart(ctx, app)
	}

	return runNomadRestart(ctx, app)
}

func runNomadRestart(ctx context.Context, app *api.AppCompact) error {
	client := client.FromContext(ctx).API()

	if _, err := client.RestartApp(ctx, app.Name); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", app.Name)

	return nil
}

func runMachinesRestart(ctx context.Context, app *api.AppCompact) error {

	input := &api.RestartMachineInput{
		ForceStop:        flag.GetBool(ctx, "force-stop"),
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
	}

	// Rolling restart against exclusively the machines managed by the Apps platform
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	machines, releaseFunc, err := machine.AcquireLeases(ctx, machines)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	for _, m := range machines {
		if err := machine.Restart(ctx, m, input); err != nil {
			return err
		}
	}

	return nil

}
