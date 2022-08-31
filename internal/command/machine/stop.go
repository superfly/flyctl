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
		args    = flag.Args(ctx)
		signal  = flag.GetString(ctx, "signal")
		timeout = flag.GetInt(ctx, "time")
	)

	if err = Stop(ctx, args, signal, timeout); err != nil {
		return
	}
	return
}

func Stop(ctx context.Context, machines []string, sig string, timeOut int) (err error) {
	var (
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
	)

	for _, arg := range machines {
		signal := api.Signal{}
		if sig != "" {
			s, err := strconv.Atoi(sig)
			if err != nil {
				return fmt.Errorf("could not get signal %s", err)
			}
			signal.Signal = syscall.Signal(s)
		}
		machineStopInput := api.StopMachineInput{
			ID:      arg,
			Signal:  signal,
			Timeout: time.Duration(timeOut),
			Filters: &api.Filters{},
		}

		app, err := appFromMachineOrName(ctx, arg, appName)
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
		fmt.Fprintf(out, "%s has been successfully stopped\n", machineStopInput.ID)
	}
	return
}
