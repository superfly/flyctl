package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStart() *cobra.Command {
	const (
		short = "Start one or more Fly machines"
		long  = short + "\n"

		usage = "start <id> [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineStart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runMachineStart(ctx context.Context) (err error) {
	var args = flag.Args(ctx)

	if err = Start(ctx, args); err != nil {
		return
	}
	return
}

func Start(ctx context.Context, machineIDs []string) (err error) {
	var (
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
	)

	for _, machineID := range machineIDs {
		app, err := appFromMachineOrName(ctx, machineID, appName)
		if err != nil {
			return fmt.Errorf("could not make flaps client: %w", err)
		}

		flapsClient, err := flaps.New(ctx, app)
		if err != nil {
			return fmt.Errorf("could not make flaps client: %w", err)
		}

		machine, err := flapsClient.Start(ctx, machineID)
		if err != nil {
			return fmt.Errorf("could not start machine %s: %w", machineID, err)
		}

		if machine.Status == "error" {
			return fmt.Errorf("machine could not be started %s", machine.Message)
		}
		fmt.Fprintf(out, "%s has been started\n", machineID)
	}
	return
}
