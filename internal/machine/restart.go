package machine

import (
	"context"
	"fmt"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func Restart(ctx context.Context, m *fly.Machine, input *fly.RestartMachineInput, nonce string) error {
	var (
		flapsClient = flapsutil.ClientFromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
	)

	fmt.Fprintf(io.Out, "Restarting machine %s\n", colorize.Bold(m.ID))
	input.ID = m.ID
	if err := flapsClient.Restart(ctx, *input, nonce); err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	if err := WaitForStartOrStop(ctx, &fly.Machine{ID: input.ID}, "start", time.Minute*5); err != nil {
		return err
	}

	if !input.SkipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*fly.Machine{m}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}
	fmt.Fprintf(io.Out, "Machine %s restarted successfully!\n", colorize.Bold(m.ID))

	return nil
}
