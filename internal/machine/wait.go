package machine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
)

func WaitForStartOrStop(ctx context.Context, machine *api.Machine, action string, timeout time.Duration) error {
	var flapsClient = flaps.FromContext(ctx)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var waitOnAction string
	switch action {
	case "start":
		waitOnAction = "started"
	case "stop":
		waitOnAction = "stopped"
	default:
		return fmt.Errorf("action must be either start or stop")
	}

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: false,
	}
	for {
		err := flapsClient.Wait(waitCtx, machine, waitOnAction, 60*time.Second)
		switch {
		case errors.Is(err, context.Canceled):
			return err
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("timeout reached waiting for machine to %s %w", waitOnAction, err)
		case err != nil:
			time.Sleep(b.Duration())
			continue
		}
		return nil
	}
}
