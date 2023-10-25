package machine

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
)

func newUpdate() *cobra.Command {
	const (
		short = "Update a machine"
		long  = short + "\n"

		usage = "update [machine_id]"
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
			Name:        "skip-start",
			Description: "Updates machine without starting it.",
			Default:     false,
		},
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
		flag.String{
			Name:        "mount-point",
			Description: "New volume mount point",
		},
		flag.Int{
			Name:        "wait-timeout",
			Description: "Seconds to wait for individual machines to transition states and become healthy. (default 300)",
			Default:     300,
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
		skipStart        = flag.GetBool(ctx, "skip-start")
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
	}

	// Identify configuration changes
	machineConf, err := determineMachineConfig(ctx, &determineMachineConfigInput{
		initialMachineConf: *machine.Config,
		appName:            appName,
		imageOrPath:        imageOrPath,
		region:             machine.Region,
		updating:           true,
	})
	if err != nil {
		return err
	}

	if mp := flag.GetString(ctx, "mount-point"); mp != "" {
		if len(machineConf.Mounts) != 1 {
			return fmt.Errorf("Machine doesn't have a volume attached")
		}
		machineConf.Mounts[0].Path = mp
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
		Name:             machine.Name,
		Region:           machine.Region,
		Config:           machineConf,
		SkipLaunch:       len(machineConf.Standbys) > 0 || skipStart,
		SkipHealthChecks: skipHealthChecks,
		Timeout:          flag.GetInt(ctx, "wait-timeout"),
	}
	if err := mach.Update(ctx, machine, input); err != nil {
		var timeoutErr mach.WaitTimeoutErr
		if errors.As(err, &timeoutErr) {
			return flyerr.GenericErr{
				Err:      timeoutErr.Error(),
				Descript: timeoutErr.Description(),
				Suggest:  "Try increasing the --wait-timeout",
			}

		}
		return err
	}

	if !(input.SkipLaunch || flag.GetDetach(ctx)) {
		fmt.Fprintln(io.Out, colorize.Green("==> "+"Monitoring health checks"))

		if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "\nMonitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", appName, machine.ID)

	return nil
}
