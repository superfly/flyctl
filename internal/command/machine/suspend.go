package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newSuspend() *cobra.Command {
	const (
		short = "Suspend one or more Fly machines"
		long  = short + "\n"

		usage = "suspend [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineSuspend,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.Duration{
			Name:        "wait-timeout",
			Shorthand:   "w",
			Description: "Duration to wait for individual Machines to be suspended.",
			Default:     0 * time.Second,
		},
	)

	return cmd
}

func runMachineSuspend(ctx context.Context) (err error) {
	var (
		io          = iostreams.FromContext(ctx)
		args        = flag.Args(ctx)
		waitTimeout = flag.GetDuration(ctx, "wait-timeout")
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

	for _, machine := range machines {
		if err = suspend(ctx, machine, waitTimeout); err != nil {
			return
		}
		if waitTimeout != 0 {
			fmt.Fprintf(io.Out, "%s has been suspended\n", machine.ID)
		} else {
			fmt.Fprintf(io.Out, "%s is being suspended\n", machine.ID)
		}
	}
	return
}

func suspend(ctx context.Context, machine *fly.Machine, waitTimeout time.Duration) error {
	client := flapsutil.ClientFromContext(ctx)
	if err := client.Suspend(ctx, machine.ID, machine.LeaseNonce); err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, machine.ID); err != nil {
			return err
		}
		return fmt.Errorf("could not suspend Machine %s: %w", machine.ID, err)
	}

	if waitTimeout != 0 {
		machine, err := client.Get(ctx, machine.ID)
		if err != nil {
			return fmt.Errorf("could not get Machine %s to wait for suspension: %w", machine.ID, err)
		}
		err = client.Wait(ctx, machine, "suspended", waitTimeout)
		if err != nil {
			return fmt.Errorf("Machine %s was not suspended within the wait timeout: %w", machine.ID, err)
		}
	}

	return nil
}
