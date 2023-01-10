package machine

import (
	"context"
	"fmt"
	"strings"

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
	var (
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
	)

	for _, machineID := range args {
		if err = Start(ctx, machineID); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been started\n", machineID)
	}
	return
}

func Start(ctx context.Context, machineID string) (err error) {
	var (
		appName = app.NameFromContext(ctx)
	)

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
		switch {
		case strings.Contains(err.Error(), "not found") && appName != "":
			return fmt.Errorf("machine %s was not found in app %s", machineID, appName)
		default:
			return fmt.Errorf("could not start machine %s: %w", machineID, err)
		}
	}

	if machine.Status == "error" {
		return fmt.Errorf("machine could not be started %s", machine.Message)
	}
	return
}
