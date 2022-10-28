package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
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
		out      = io.Out
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
	machineConf, machineDiff, err := determineMachineConfig(ctx, machineConf, app, imageOrPath)
	if err != nil {
		return
	}

	s := strings.Split(machineDiff, ",")
	var str string
	for _, val := range s {
		_, foundDeletion := presenters.GetStringInBetweenTwoString(val, "-", ":")
		_, foundAddition := presenters.GetStringInBetweenTwoString(val, "+", ":")
		if foundAddition {
			str += colorize.Green(val)
		} else if foundDeletion {
			str += colorize.Red(val)
		} else {
			str += val
		}
	}
	fmt.Fprintln(out, "The following config is being updated")
	fmt.Fprintln(out, str)

	// interactive update //
	name := false
	prompt := &survey.Confirm{
		Message: "Confirm update?",
	}
	survey.AskOne(prompt, &name)
	if !name {
		fmt.Fprintln(out, colorize.Yellow(fmt.Sprintf("\nCancelling update to machine %s\n", machine.ID)))
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

	fmt.Fprintln(out, colorize.Yellow(fmt.Sprintf("Machine %s has been updated\n", machine.ID)))
	fmt.Fprintf(out, "Instance ID has been updated:\n")
	fmt.Fprintf(out, "%s -> %s\n\n", prevInstanceID, machine.InstanceID)

	//wait for machine to be started
	if err := WaitForStartOrStop(ctx, machine, waitForAction, time.Second*60); err != nil {
		return err
	}

	fmt.Fprintf(out, "Image: %s\n", machineConf.Image)

	if waitForAction == "start" {
		fmt.Fprintf(out, "State: Started\n\n")
	} else {
		fmt.Fprintf(out, "State: Stopped\n\n")
	}

	fmt.Fprintf(out, "\nMonitor machine status here:\nhttps://fly.io/apps/%s/machines/%s\n", app.Name, machine.ID)

	return nil
}
