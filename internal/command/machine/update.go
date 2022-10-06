package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		appName  = app.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	machineID := flag.FirstArg(ctx)

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return err
	}

	imageOrPath := machine.Config.Image
	image := flag.GetString(ctx, flag.ImageName)
	dockerfile := flag.GetString(ctx, flag.Dockerfile().Name)
	if len(image) > 0 {
		imageOrPath = image
	} else if len(dockerfile) > 0 {
		imageOrPath = "." // cwd
	}

	prevInstanceID := machine.InstanceID

	fmt.Fprintf(io.Out, "Machine %s was found and is currently in a %s state, attempting to update...\n", machineID, machine.State)

	machineConf := *machine.Config
	machineConf, err = determineMachineConfig(ctx, machineConf, app, imageOrPath)
	if err != nil {
		return
	}

	input := api.LaunchMachineInput{
		ID:     machine.ID,
		AppID:  app.Name,
		Name:   machine.Name,
		Region: machine.Region,
		Config: &machineConf,
	}

	machine, err = flapsClient.Update(ctx, input, "")
	if err != nil {
		return err
	}

	waitForAction := "start"
	if machine.Config.Schedule != "" {
		waitForAction = "stop"
	}

	out := io.Out
	fmt.Fprintln(out, colorize.Yellow(fmt.Sprintf("Machine %s has been updated\n", machine.ID)))
	fmt.Fprintf(out, "Instance ID has been updated:\n")
	fmt.Fprintf(out, "%s -> %s\n\n", prevInstanceID, machine.InstanceID)

	// wait for machine to be started
	if err := WaitForStartOrStop(ctx, machine, waitForAction, time.Second*60); err != nil {
		return err
	}

	fmt.Fprintf(out, "Image: %s\n", machineConf.Image)

	if waitForAction == "start" {
		fmt.Fprintf(out, "State: Started\n\n")
	} else {
		fmt.Fprintf(out, "State: Stopped\n\n")
	}

	fmt.Fprintf(out, "Monitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", app.Name, machine.ID)

	return nil
}
