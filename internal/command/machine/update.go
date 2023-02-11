package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
)

func newUpdate() *cobra.Command {
	const (
		short = "Update a machine"
		long  = short + "\n"

		usage = "update <machine_id>"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.Image(),
		sharedFlags,
		flag.Yes(),
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Updates machine without waiting for health checks.",
			Default:     false,
		},
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		appName  = app.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		machineID        = flag.FirstArg(ctx)
		autoConfirm      = flag.GetBool(ctx, "yes")
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	// Get machine

	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return err
	}

	// Acquire lease
	machine, releaseLeaseFunc, err := mach.AcquireLease(ctx, machine)
	defer releaseLeaseFunc(ctx, machine)
	if err != nil {
		return err
	}

	// Resolve image
	imageOrPath := machine.Config.Image
	image := flag.GetString(ctx, flag.ImageName)
	dockerfile := flag.GetString(ctx, flag.Dockerfile().Name)
	if len(image) > 0 {
		imageOrPath = image
	} else if len(dockerfile) > 0 {
		imageOrPath = "." // cwd
	}

	// Identify configuration changes
	machineConf, err := determineMachineConfig(ctx, *machine.Config, app, imageOrPath, machine.Region)
	if err != nil {
		return err
	}

	// Prompt user to confirm changes
	if !autoConfirm {
		confirmed, err := mach.ConfirmConfigChanges(ctx, machine, *machineConf, "")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintf(io.Out, "No changes to apply\n")
			return nil
		}
	}

	// Perform update
	input := &api.LaunchMachineInput{
		ID:               machine.ID,
		AppID:            app.Name,
		Name:             machine.Name,
		Region:           machine.Region,
		Config:           machineConf,
		SkipHealthChecks: skipHealthChecks,
	}
	if err := mach.Update(ctx, machine, input); err != nil {
		return err
	}

	if !flag.GetDetach(ctx) {
		fmt.Fprintln(io.Out, colorize.Green("==> "+"Monitoring health checks"))

		if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "\nMonitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", app.Name, machine.ID)

	return nil
}
