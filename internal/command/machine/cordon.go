package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newMachineCordon() *cobra.Command {
	const (
		short = "Deactivate all services on a machine"
		long  = short + "\n"
		usage = "cordon [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineCordon,
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

func runMachineCordon(ctx context.Context) (err error) {
	var (
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
	)

	machines, ctx, err := selectManyMachines(ctx, args)
	if err != nil {
		return err
	}

	machines, release, err := mach.AcquireLeases(ctx, machines)
	defer release()
	if err != nil {
		return err
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	for _, machine := range machines {
		fmt.Fprintf(io.Out, "Activating cordon on machine %s...\n", machine.ID)
		if err = flapsClient.Cordon(ctx, machine.ID, machine.LeaseNonce); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "done!\n")
	}
	return
}
