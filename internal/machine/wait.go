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
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/trace"
)

func WaitForState(ctx context.Context, appName string, machine *fly.Machine, desiredState string, timeout time.Duration) (string, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)

	if err := WaitForStartOrStop(ctx, appName, machine, desiredState, timeout); err != nil {
		return "", err
	}
	if desiredState == "settled" {
		m, err := flapsClient.Get(ctx, appName, machine.ID)
		if err != nil {
			return "", fmt.Errorf("failed to get machine after waiting for it to settle: %w", err)
		}

		return m.State, nil
	}

	return desiredState, nil
}

// WaitForLaunch waits for the first start of a newly created Machine.
func WaitForLaunch(ctx context.Context, appName string, machine *fly.Machine, timeout time.Duration) error {
	for _, event := range machine.Events {
		if event != nil && event.Type == "launch" && event.Status == "created" && event.ID != "" {
			return waitForStartOrStop(ctx, appName, machine, "start", timeout, event.ID)
		}
	}
	return WaitForStartOrStop(ctx, appName, machine, "start", timeout)
}

func WaitForStartOrStop(ctx context.Context, appName string, machine *fly.Machine, action string, timeout time.Duration) error {
	return waitForStartOrStop(ctx, appName, machine, action, timeout, "")
}

func waitForStartOrStop(ctx context.Context, appName string, machine *fly.Machine, action string, timeout time.Duration, fromEventID string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)

	waitCtx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("waiting for machine start or stop: %w", context.DeadlineExceeded))
	defer cancel()
	defer func() { tracing.RecordCancellation(waitCtx, trace.SpanFromContext(waitCtx)) }()

	var waitOnAction string
	switch action {
	case "start":
		waitOnAction = "started"
	case "stop":
		waitOnAction = "stopped"
	case "settled":
		waitOnAction = "settled"
	default:
		return invalidAction
	}

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: false,
	}
	waitOpts := []flaps.WaitOption{flaps.WithWaitStates(waitOnAction), flaps.WithWaitTimeout(time.Minute)}
	if fromEventID != "" {
		waitOpts = append(waitOpts, flaps.WithWaitFromEventID(fromEventID))
	}
	waitingForFirstStart := false
	for {
		err := flapsClient.Wait(waitCtx, appName, machine.ID, waitOpts...)
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
			isFlapsError := errors.As(err, &flapsErr)
			if waitingForFirstStart && isFlapsError && (flapsErr.ResponseStatusCode == http.StatusConflict || flapsErr.ResponseStatusCode == http.StatusNotFound) {
				return err
			}
			if strings.Contains(err.Error(), "machine failed to reach desired state") && machine.Config != nil && machine.Config.Restart != nil && machine.Config.Restart.Policy == fly.MachineRestartPolicyNo {
				if action != "start" || machine.Version == "" || !isFlapsError || flapsErr.ResponseStatusCode != http.StatusRequestTimeout ||
					flapsErr.Error() != "deadline_exceeded: machine failed to reach desired state, started, currently created" {
					return fmt.Errorf("machine failed to reach desired start state, and restart policy was set to %s restart", machine.Config.Restart.Policy)
				}
				// Follow the original launch if another version replaces this one.
				if !waitingForFirstStart {
					waitOpts = append(waitOpts, flaps.WithWaitVersion(machine.Version))
					waitingForFirstStart = true
				}
			}
			if isFlapsError && flapsErr.ResponseStatusCode == http.StatusBadRequest {
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
