package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func RollingRestart(ctx context.Context, input *api.RestartMachineInput) error {
	machines, releaseFunc, err := AcquireAllLeases(ctx)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	for _, m := range machines {
		Restart(ctx, m, input)
	}

	return nil
}

func Restart(ctx context.Context, m *api.Machine, input *api.RestartMachineInput) error {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
	)

	fmt.Fprintf(io.Out, "Restarting machine %s\n", colorize.Bold(m.ID))
	input.ID = m.ID
	if err := flapsClient.Restart(ctx, *input); err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	if err := WaitForStartOrStop(ctx, &api.Machine{ID: input.ID}, "start", time.Minute*5); err != nil {
		return err
	}

	if !input.SkipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*api.Machine{m}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}
	fmt.Fprintf(io.Out, "Machine %s restarted successfully!\n", colorize.Bold(m.ID))

	return nil
}
