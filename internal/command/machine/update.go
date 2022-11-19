package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
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
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Auto-confirm changes",
		},
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
		client  = client.FromContext(ctx).API()

		machineID = flag.FirstArg(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	flapsClient := flaps.FromContext(ctx)

	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return err
	}

	// Acquire lease
	machine, err = mach.AcquireLease(ctx, machine)
	if err != nil {
		return err
	}
	defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)

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
	machineConf, err := determineMachineConfig(ctx, *machine.Config, app, imageOrPath)
	if err != nil {
		return err
	}

	confirmed, err := mach.ConfirmConfigChanges(ctx, machine, *machineConf, "")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintf(io.Out, "No changes to apply\n")
		return nil
	}

	input := &api.LaunchMachineInput{
		ID:     machine.ID,
		AppID:  app.Name,
		Name:   machine.Name,
		Region: machine.Region,
		Config: machineConf,
	}

	if err := mach.Update(ctx, machine, input); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nMonitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", app.Name, machine.ID)

	return nil
}
