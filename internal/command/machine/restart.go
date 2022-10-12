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

func newRestart() *cobra.Command {
	const (
		short = "Restart one or more Fly machines"
		long  = short + "\n"

		usage = "restart <id> [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineRestart,
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
		flag.Bool{
			Name:        "force",
			Description: "Force stop the machine(s)",
		},
	)

	return cmd
}

func runMachineRestart(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		signal  = flag.GetString(ctx, "signal")
		timeout = flag.GetInt(ctx, "time")
	)

	var forceStop = false

	if flag.GetBool(ctx, "force") {
		forceStop = true
	}

	for _, machineID := range args {
		fmt.Fprintf(io.Out, "Sending kill signal to machine %s...", machineID)

		if err = Restart(ctx, machineID, signal, timeout, forceStop); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been successfully stopped\n", machineID)
	}
	return
}

func Restart(ctx context.Context, machineID, sig string, timeOut int, forceStop bool) (err error) {
	var (
		appName = app.NameFromContext(ctx)
	)

	input := api.RestartMachineInput{
		ID:        machineID,
		Timeout:   time.Duration(timeOut),
		ForceStop: forceStop,
	}

	if sig != "" {
		signal := &api.Signal{}

		s, err := strconv.Atoi(sig)
		if err != nil {
			return fmt.Errorf("could not get signal %s", err)
		}
		signal.Signal = syscall.Signal(s)
		input.Signal = signal
	}

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	err = flapsClient.Restart(ctx, input)
	if err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	ctx = flaps.NewContext(ctx, flapsClient)
	if err = WaitForStartOrStop(ctx, &api.Machine{ID: input.ID}, "start", time.Minute*5); err != nil {
		return
	}

	return
}
