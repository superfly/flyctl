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

func Update(ctx context.Context, m *api.Machine, input *api.LaunchMachineInput) error {
	var (
		flapsClient    = flaps.FromContext(ctx)
		io             = iostreams.FromContext(ctx)
		colorize       = io.ColorScheme()
		updatedMachine *api.Machine
		err            error
	)

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	updatedMachine, err = flapsClient.Update(ctx, *input, m.LeaseNonce)
	if err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	waitForAction := "start"
	if m.Config.Schedule != "" {
		waitForAction = "stop"
	}

	if err := WaitForStartOrStop(ctx, updatedMachine, waitForAction, time.Minute*5); err != nil {
		return err
	}

	if !input.SkipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*api.Machine{updatedMachine}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
}
