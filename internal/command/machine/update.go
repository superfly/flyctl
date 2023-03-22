package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
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
		selectFlag,
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Updates machine without waiting for health checks.",
			Default:     false,
		},
		flag.String{
			Name:        "command",
			Shorthand:   "C",
			Description: "Command to run",
		},
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		autoConfirm      = flag.GetBool(ctx, "yes")
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
		image            = flag.GetString(ctx, "image")
		dockerfile       = flag.GetString(ctx, flag.Dockerfile().Name)
	)

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	machine, ctx, err := selectOneMachine(ctx, nil, machineID, haveMachineID)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	// Acquire lease
	machine, releaseLeaseFunc, err := mach.AcquireLease(ctx, machine)
	defer releaseLeaseFunc(ctx, machine)
	if err != nil {
		return err
	}

	var imageOrPath string

	if image != "" {
		imageOrPath = image
	} else if dockerfile != "" {
		imageOrPath = "."
	} else {
		imageOrPath = machine.FullImageRef()
	}

	if imageOrPath == "" {
		return fmt.Errorf("failed to resolve machine image")
	}

	// Identify configuration changes
	machineConf, err := determineMachineConfig(ctx, *machine.Config, appName, imageOrPath, machine.Region)
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
		AppID:            appName,
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

	fmt.Fprintf(io.Out, "\nMonitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", appName, machine.ID)

	return nil
}
