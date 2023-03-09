package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
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
	)

	return cmd
}

func runMachineStop(ctx context.Context) (err error) {
	var (
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
	)

	machineIDs, ctx, err := selectManyMachineIDs(ctx, args)
	if err != nil {
		return err
	}

	for _, machineID := range machineIDs {
		fmt.Fprintf(io.Out, "Sending kill signal to machine %s...\n", machineID)

		if err = Stop(ctx, machineID); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been successfully stopped\n", machineID)
	}
	return
}

func Stop(ctx context.Context, machineID string) (err error) {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

	machineStopInput := api.StopMachineInput{
		ID:      machineID,
		Filters: &api.Filters{},
	}

	err = flaps.FromContext(ctx).Stop(ctx, machineStopInput)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return fmt.Errorf("machine %s was not found in app '%s'", machineID, appName)
		default:
			return fmt.Errorf("could not stop machine %s: %w", machineStopInput.ID, err)
		}
	}

	return
}
