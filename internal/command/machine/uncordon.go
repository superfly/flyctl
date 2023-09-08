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

func newMachineUncordon() *cobra.Command {
	const (
		short = "Reactive all services on a machine"
		long  = short + "\n"
		usage = "uncordon <id> [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineUncordon,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
	)

	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func runMachineUncordon(ctx context.Context) (err error) {
	var (
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
	)

	machineIDs, ctx, err := selectManyMachineIDs(ctx, args)
	if err != nil {
		return err
	}

	flapsClient := flaps.FromContext(ctx)

	for _, machineID := range machineIDs {
		fmt.Fprintf(io.Out, "Deactivating cordon on machine %s...\n", machineID)
		if err = flapsClient.Uncordon(ctx, machineID); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "done!\n")
	}
	return
}
