package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type EphemeralInput struct {
	LaunchInput fly.LaunchMachineInput
	What        string
}

func LaunchEphemeral(ctx context.Context, input *EphemeralInput) (*fly.Machine, func(), error) {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flapsutil.ClientFromContext(ctx)
	)

	if !input.LaunchInput.Config.AutoDestroy {
		return nil, nil, errors.New("ephemeral machines must be configured to auto-destroy (this is a bug)")
	}

	machine, err := flapsutil.Launch(ctx, flapsClient, input.LaunchInput)
	if err != nil {
		return nil, nil, err
	}

	if cmdutil.IsTerminal(os.Stdout) {
		creationMsg := "Created an ephemeral machine " + colorize.Bold(machine.ID)
		if input.What != "" {
			creationMsg += " " + input.What
		}
		fmt.Fprintf(io.Out, "%s.\n", creationMsg)

		sp := spinner.Run(io, fmt.Sprintf("Waiting for %s to start ...", colorize.Bold(machine.ID)))
		defer sp.Stop()
	}

	const waitTimeout = 15 * time.Second
	var flapsErr *flaps.FlapsError

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		err = flapsClient.Wait(ctx, machine, fly.MachineStateStarted, waitTimeout)
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

func checkMachineDestruction(ctx context.Context, machine *fly.Machine, firstErr error) (bool, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	machine, err := flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return false, fmt.Errorf("failed to check status of machine: %w", err)
	}

	if machine.State != fly.MachineStateDestroyed && machine.State != fly.MachineStateDestroying {
		return false, firstErr
	}

	var exitEvent *fly.MachineEvent
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

func makeCleanupFunc(ctx context.Context, machine *fly.Machine) func() {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flapsutil.ClientFromContext(ctx)
	)

	return func() {
		const stopTimeout = 15 * time.Second

		// FIXME: is there a reason we *need* to use context.Background here, instead of the normal context
		// As far as I can tell, this is the only place in the codebase that does this
		stopCtx, cancel := context.WithTimeout(ctx, stopTimeout)
		stopCtx, cancel = ctrlc.HookCancelableContext(stopCtx, cancel)
		defer cancel()

		stopInput := fly.StopMachineInput{
			ID:      machine.ID,
			Timeout: fly.Duration{Duration: stopTimeout},
		}
		if err := flapsClient.Stop(stopCtx, stopInput, ""); err != nil {
			terminal.Warnf("Failed to stop ephemeral machine: %v", err)
			terminal.Warn("You may need to destroy it manually (`fly machine destroy`).")
			return
		}

		if cmdutil.IsTerminal(os.Stdout) {
			fmt.Fprintf(io.Out, "Waiting for ephemeral machine %s to be destroyed ...", colorize.Bold(machine.ID))
			if err := flapsClient.Wait(stopCtx, machine, fly.MachineStateDestroyed, stopTimeout); err != nil {
				fmt.Fprintf(io.Out, " %s!\n", colorize.Red("failed"))
				terminal.Warnf("Failed to wait for ephemeral machine to be destroyed: %v", err)
				terminal.Warn("You may need to destroy it manually (`fly machine destroy`).")
			} else {
				fmt.Fprintf(io.Out, " %s.\n", colorize.Green("done"))
			}
		} else {
			if err := flapsClient.Wait(stopCtx, machine, fly.MachineStateDestroyed, stopTimeout); err != nil {
				fmt.Fprintf(io.ErrOut, "Attempt to destroy ephemeral machine %s failed: %v", machine.ID, err)
				fmt.Fprint(io.ErrOut, "You may need to destroy it manually (`fly machine destroy`).")
			}
		}
	}
}
