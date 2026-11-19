package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart one or more Fly Machines"
		long  = `Restart one or more Fly Machines, one at a time. Pass machine IDs as
arguments; the app is looked up from the first ID unless ` + "`--app`" + ` or ` + "`fly.toml`" + ` sets it.
With no IDs, you pick machines of the app from a list in a terminal;
without a terminal, IDs are required.

For each machine, the command waits up to five minutes for it to start and
then waits for its health checks to pass, unless you pass
` + "`--skip-health-checks`" + `. It stops at the first machine that fails.`

		usage = "restart [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineRestart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},

		flag.Int{
			Name:        "time",
			Description: "Seconds to wait before killing the machine",
		},
		flag.Bool{
			Name:        "force",
			Description: "Force stop the machine(s)",
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Restarts app without waiting for health checks.",
			Default:     false,
		},
	)

	return cmd
}

func runMachineRestart(ctx context.Context) error {
	var (
		args    = flag.Args(ctx)
		timeout = flag.GetInt(ctx, "time")
	)

	// Resolve flags
	input := &fly.RestartMachineInput{
		Timeout:          time.Duration(timeout) * time.Second,
		ForceStop:        flag.GetBool(ctx, "force"),
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
		Signal:           strings.ToUpper(flag.GetString(ctx, "signal")),
	}

	machines, ctx, err := selectManyMachines(ctx, args)
	if err != nil {
		return err
	}

	// appName is added to context by selectManyMachines
	appName := appconfig.NameFromContext(ctx)

	// Acquire leases
	machines, releaseLeaseFunc, err := mach.AcquireLeases(ctx, appName, machines)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	// Restart each machine
	for _, machine := range machines {
		if err := mach.Restart(ctx, appName, machine, input, machine.LeaseNonce); err != nil {
			return fmt.Errorf("failed to restart machine %s: %w", machine.ID, err)
		}
	}

	return nil
}
