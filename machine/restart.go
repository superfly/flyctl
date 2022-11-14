package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func RollingRestart(ctx context.Context) error {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	// Acquire leases
	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	// Requery machines to ensure we are working against the most up-to-date configuration.
	machines, err = flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	for _, m := range machines {
		Restart(ctx, m)
	}

	return nil
}

func Restart(ctx context.Context, m *api.Machine) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	skipHealthChecks := flag.GetBool(ctx, "skip-health-checks")
	forceStop := flag.GetBool(ctx, "force-stop")
	// force := flag.GetBool(ctx, "force")

	fmt.Fprintf(io.Out, "Restarting machine %s\n", colorize.Bold(m.ID))
	if err := machine.Restart(ctx, m.ID, "", 120, forceStop); err != nil {
		return err
	}

	if !skipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*api.Machine{m}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	return nil
}
