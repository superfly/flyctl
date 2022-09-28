package machine

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStop() *cobra.Command {
	const (
		short = "Stop one or more Fly machines"
		long  = short + "\n"

		usage = "stop <id> [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineStop,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},

		flag.Int{
			Name:        "time",
			Description: "Seconds to wait before killing the machine",
		},
	)

	return cmd
}

func runMachineStop(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		signal  = flag.GetString(ctx, "signal")
		timeout = flag.GetInt(ctx, "time")
	)

	for _, machineID := range args {
		fmt.Fprintf(io.Out, "Sending kill signal to machine %s...", machineID)

		if err = Stop(ctx, machineID, signal, timeout); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been successfully stopped\n", machineID)
	}
	return
}

func Stop(ctx context.Context, machineID string, sig string, timeOut int) (err error) {
	var (
		appName = app.NameFromContext(ctx)
	)

	signal := api.Signal{}
	if sig != "" {
		s, err := strconv.Atoi(sig)
		if err != nil {
			return fmt.Errorf("could not get signal %s", err)
		}
		signal.Signal = syscall.Signal(s)
	}
	machineStopInput := api.StopMachineInput{
		ID:      machineID,
		Signal:  signal,
		Timeout: time.Duration(timeOut),
		Filters: &api.Filters{},
	}

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	err = flapsClient.Stop(ctx, machineStopInput)
	if err != nil {
		return fmt.Errorf("could not stop machine %s: %w", machineStopInput.ID, err)
	}

	return
}
