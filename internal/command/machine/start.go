package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
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
		if err = Start(ctx, machine); err != nil {
			return
		}
		fmt.Fprintf(io.Out, "%s has been started\n", machine.ID)
	}
	return
}

func Start(ctx context.Context, machine *fly.Machine) (err error) {
	res, err := flapsutil.ClientFromContext(ctx).Start(ctx, machine.ID, machine.LeaseNonce)
	if err != nil {
		// TODO(dov): just do the clone
		switch {
		case strings.Contains(err.Error(), " for machine"):
			return fmt.Errorf("could not start machine due to lack of capacity. Try 'flyctl machine clone %s -a %s'", machine.ID, appconfig.NameFromContext(ctx))
		default:
			if err := rewriteMachineNotFoundErrors(ctx, err, machine.ID); err != nil {
				return err
			}
			return fmt.Errorf("could not start machine %s: %w", machine.ID, err)
		}
	}

	if res.Status == "error" {
		return fmt.Errorf("machine could not be started: %s", res.Message)
	}
	return
}
