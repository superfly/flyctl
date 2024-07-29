package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
)

func WaitForStartOrStop(ctx context.Context, machine *fly.Machine, action string, timeout time.Duration) error {
	flapsClient := flapsutil.ClientFromContext(ctx)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var waitOnAction string
	switch action {
	case "start":
		waitOnAction = "started"
	case "stop":
		waitOnAction = "stopped"
	default:
		return invalidAction
	}

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: false,
	}
	for {
		err := flapsClient.Wait(waitCtx, machine, waitOnAction, 60*time.Second)
		if err == nil {
			return nil
		}

		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return WaitTimeoutErr{
				machineID:    machine.ID,
				timeout:      timeout,
				desiredState: waitOnAction,
			}
		default:
			var flapsErr *flaps.FlapsError
			if strings.Contains(err.Error(), "machine failed to reach desired state") && machine.Config.Restart != nil && machine.Config.Restart.Policy == fly.MachineRestartPolicyNo {
				return fmt.Errorf("machine failed to reach desired start state, and restart policy was set to %s restart", machine.Config.Restart.Policy)
			}
			if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusBadRequest {
				return fmt.Errorf("failed waiting for machine: %w", err)
			}
			time.Sleep(b.Duration())
		}
	}
}

type WaitTimeoutErr struct {
	machineID    string
	timeout      time.Duration
	desiredState string
}

func (e WaitTimeoutErr) Error() string {
	return "timeout reached waiting for machine's state to change"
}

func (e WaitTimeoutErr) Description() string {
	return fmt.Sprintf("The machine %s took more than %s to reach \"%s\"", e.machineID, e.timeout, e.desiredState)
}

func (e WaitTimeoutErr) DesiredState() string {
	return e.desiredState
}

var invalidAction flyerr.GenericErr = flyerr.GenericErr{
	Err:      "action must be either start or stop",
	Descript: "",
	Suggest:  "This is a bug in wait function, please report this at https://community.fly.io",
	DocUrl:   "",
}
