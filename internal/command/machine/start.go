package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
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

	machineIDs, ctx, err := selectManyMachineIDs(ctx, args)
	if err != nil {
		return err
	}

	for _, machineID := range machineIDs {
		if err = Start(ctx, machineID); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been started\n", machineID)
	}
	return
}

func Start(ctx context.Context, machineID string) (err error) {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

	machine, err := flaps.FromContext(ctx).Start(ctx, machineID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return fmt.Errorf("machine %s was not found in app '%s'", machineID, appName)
		default:
			return fmt.Errorf("could not start machine %s: %w", machineID, err)
		}
	}

	if machine.Status == "error" {
		return fmt.Errorf("machine could not be started: %s", machine.Message)
	}
	return
}
