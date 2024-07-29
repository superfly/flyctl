package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func newKill() *cobra.Command {
	const (
		short = "Kill (SIGKILL) a Fly machine"
		long  = short + "\n"

		usage = "kill [id]"
	)

	cmd := command.New(usage, short, long, runMachineKill,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
	)

	return cmd
}

func runMachineKill(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	current, ctx, err := selectOneMachine(ctx, "", machineID, haveMachineID)
	if err != nil {
		return err
	}
	flapsClient := flapsutil.ClientFromContext(ctx)

	if current.State == "destroyed" {
		return fmt.Errorf("machine %s has already been destroyed", current.ID)
	}
	fmt.Fprintf(io.Out, "machine %s was found and is currently in a %s state, attempting to kill...\n", current.ID, current.State)

	err = flapsClient.Kill(ctx, current.ID)
	if err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, current.ID); err != nil {
			return err
		}
		return fmt.Errorf("could not kill machine %s: %w", current.ID, err)
	}

	fmt.Fprintln(io.Out, "kill signal has been sent")

	return nil
}
