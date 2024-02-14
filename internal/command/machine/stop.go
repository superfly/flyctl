package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStop() *cobra.Command {
	const (
		short = "Stop one or more Fly machines"
		long  = short + "\n"

		usage = "stop [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineStop,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},
		flag.Int{
			Name:        "timeout",
			Description: "Seconds to wait before sending SIGKILL to the machine",
		},
		flag.Duration{
			Name:        "wait-timeout",
			Shorthand:   "w",
			Description: "Time duration to wait for individual machines to transition states and become stopped.",
			Default:     0 * time.Second,
		},
	)

	return cmd
}

func runMachineStop(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		signal  = flag.GetString(ctx, "signal")
		timeout = flag.GetInt(ctx, "timeout")
	)

	machineIDs, ctx, err := selectManyMachineIDs(ctx, args)
	if err != nil {
		return err
	}

	for _, machineID := range machineIDs {
		fmt.Fprintf(io.Out, "Sending kill signal to machine %s...\n", machineID)

		if err = Stop(ctx, machineID, signal, timeout); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been successfully stopped\n", machineID)
	}
	return
}

func Stop(ctx context.Context, machineID string, signal string, timeout int) (err error) {
	machineStopInput := fly.StopMachineInput{
		ID:     machineID,
		Signal: strings.ToUpper(signal),
	}

	if timeout > 0 {
		machineStopInput.Timeout = fly.Duration{Duration: time.Duration(timeout) * time.Second}
	}

	waitTimeout := flag.GetDuration(ctx, "wait-timeout")

	client := flaps.FromContext(ctx)
	err = client.Stop(ctx, machineStopInput, "")
	if err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
			return err
		}
		return fmt.Errorf("could not stop machine %s: %w", machineID, err)
	}

	if waitTimeout != 0 {
		machine, err := client.Get(ctx, machineID)
		if err != nil {
			return fmt.Errorf("could not get machine %s to wait for stop: %w", machineID, err)
		}
		err = client.Wait(ctx, machine, "stopped", waitTimeout)
		if err != nil {
			return fmt.Errorf("machine %s did not stop within the wait timeout: %w", machineID, err)
		}
	}

	return
}
