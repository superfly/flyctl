package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type EphemeralInput struct {
	LaunchInput api.LaunchMachineInput
	What        string
}

func LaunchEphemeral(ctx context.Context, input *EphemeralInput) (*api.Machine, func(), error) {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flaps.FromContext(ctx)
	)

	if !input.LaunchInput.Config.AutoDestroy {
		return nil, nil, errors.New("ephemeral machines must be configured to auto-destroy (this is a bug)")
	}

	machine, err := flapsClient.Launch(ctx, input.LaunchInput)
	if err != nil {
		return nil, nil, err
	}

	creationMsg := "Created an ephemeral machine " + colorize.Bold(machine.ID)
	if input.What != "" {
		creationMsg += " " + input.What
	}
	fmt.Fprintf(io.Out, "%s.\n", creationMsg)

	sp := spinner.Run(io, fmt.Sprintf("Waiting for %s to start ...", machine.ID))
	defer sp.Stop()

	const waitTimeout = 15 * time.Second
	var flapsErr *flaps.FlapsError

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		err = flapsClient.Wait(ctx, machine, api.MachineStateStarted, waitTimeout)
		if err == nil {
			return machine, makeCleanupFunc(ctx, machine), nil
		}

		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusRequestTimeout {
			// The machine may not be ready yet.
		} else {
			break
		}

		select {
		case <-ctx.Done():
			terminal.Warn("You may need to destroy the machine manually (`fly machine destroy`).")
			return nil, nil, ctx.Err()
		case <-t.C:
		}
	}

	var destroyed bool
	if flapsErr != nil && flapsErr.ResponseStatusCode == http.StatusNotFound {
		destroyed, err = checkMachineDestruction(ctx, machine, err)
	}

	if !destroyed {
		terminal.Warn("You may need to destroy the machine manually (`fly machine destroy`).")
	}
	return nil, nil, err
}

func checkMachineDestruction(ctx context.Context, machine *api.Machine, firstErr error) (bool, error) {
	flapsClient := flaps.FromContext(ctx)
	machine, err := flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return false, fmt.Errorf("failed to check status of machine: %w", err)
	}

	if machine.State != api.MachineStateDestroyed && machine.State != api.MachineStateDestroying {
		return false, firstErr
	}

	var exitEvent *api.MachineEvent
	for _, event := range machine.Events {
		if event.Type == "exit" {
			exitEvent = event
			break
		}
	}

	if exitEvent == nil || exitEvent.Request == nil {
		return true, errors.New("machine was destroyed unexpectedly")
	}

	exitCode, err := exitEvent.Request.GetExitCode()
	if err != nil {
		return true, errors.New("machine exited unexpectedly")
	}

	return true, fmt.Errorf("machine exited unexpectedly with code %v", exitCode)
}

func makeCleanupFunc(ctx context.Context, machine *api.Machine) func() {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flaps.FromContext(ctx)
	)

	return func() {
		const stopTimeout = 5 * time.Second

		stopCtx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()

		stopInput := api.StopMachineInput{
			ID:      machine.ID,
			Timeout: api.Duration{Duration: stopTimeout},
		}
		if err := flapsClient.Stop(stopCtx, stopInput, ""); err != nil {
			terminal.Warnf("Failed to stop ephemeral machine: %v\n", err)
			terminal.Warn("You may need to destroy it manually (`fly machine destroy`).")
			return
		}

		fmt.Fprintf(io.Out, "Waiting for ephemeral machine %s to be destroyed ...", colorize.Bold(machine.ID))
		if err := flapsClient.Wait(stopCtx, machine, api.MachineStateDestroyed, stopTimeout); err != nil {
			fmt.Fprintf(io.Out, " %s!\n", colorize.Red("failed"))
			terminal.Warnf("Failed to wait for ephemeral machine to be destroyed: %v\n", err)
			terminal.Warn("You may need to destroy it manually (`fly machine destroy`).")
		} else {
			fmt.Fprintf(io.Out, " %s.\n", colorize.Green("done"))
		}
	}
}
