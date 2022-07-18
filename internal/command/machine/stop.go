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
		short = "Stop a Fly machine"
		long  = short + "\n"

		usage = "stop <id>"
	)

	cmd := command.New(usage, short, long, runMachineStop,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

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
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
	)

	for _, arg := range args {
		signal := api.Signal{}
		if flag.GetString(ctx, "signal") != "" {
			s, err := strconv.Atoi(flag.GetString(ctx, "signal"))
			if err != nil {
				return fmt.Errorf("could not get signal %s", err)
			}
			signal.Signal = syscall.Signal(s)
		}
		machineStopInput := api.StopMachineInput{
			ID:      arg,
			Signal:  signal,
			Timeout: time.Duration(flag.GetInt(ctx, "time")),
			Filters: &api.Filters{},
		}

		app, err := appFromMachineOrName(ctx, arg, appName)

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
