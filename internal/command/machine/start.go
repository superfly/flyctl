package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStart() *cobra.Command {
	const (
		short = "Start one or more Fly machines"
		long  = short + "\n"

		usage = "start [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineStart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
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
	machine, err := flaps.FromContext(ctx).Start(ctx, machineID, "")
	if err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
			return err
		}
		return fmt.Errorf("could not start machine %s: %w", machineID, err)
	}

	if machine.Status == "error" {
		return fmt.Errorf("machine could not be started: %s", machine.Message)
	}
	return
}
