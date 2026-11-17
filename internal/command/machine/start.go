package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/watch"
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
		flag.Bool{
			Name:        "wait",
			Description: "Wait for the Machines to start and pass configured health checks.",
		},
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

	// appName is added to context by selectManyMachines
	appName := appconfig.NameFromContext(ctx)

	machines, release, err := mach.AcquireLeases(ctx, appName, machines)
	defer release()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		if err = tracing.WorkflowPhase(ctx, "request_start", func(ctx context.Context) error {
			return Start(ctx, appName, machine)
		}); err != nil {
			return
		}
		if flag.GetBool(ctx, "wait") {
			if err = tracing.WorkflowPhase(ctx, "wait_for_start", func(ctx context.Context) error {
				return mach.WaitForStartOrStop(ctx, appName, machine, "start", 5*time.Minute)
			}); err != nil {
				return err
			}
			if len(machine.Checks) > 0 {
				if err = tracing.WorkflowPhase(ctx, "wait_for_health_checks", func(ctx context.Context) error {
					return watch.MachinesChecks(ctx, appName, []*fly.Machine{machine})
				}); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(io.Out, "%s has been started\n", machine.ID)
	}

	return
}

func Start(ctx context.Context, appName string, machine *fly.Machine) (err error) {
	res, err := flapsutil.ClientFromContext(ctx).Start(ctx, appName, machine.ID, machine.LeaseNonce)
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
