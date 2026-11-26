package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	"github.com/oklog/ulid/v2"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/trace"
)

// WaitForLaunch returns started, or destroyed if the original launch started before deletion.
func WaitForLaunch(ctx context.Context, appName string, machine *fly.Machine, timeout time.Duration) (string, error) {
	for _, event := range machine.Events {
		if event != nil && event.Type == "launch" && event.Status == "created" && event.ID != "" {
			return waitForStartOrStop(ctx, appName, machine, "start", timeout, event.ID, "")
		}
	}

	return waitForStartOrStop(ctx, appName, machine, "start", timeout, "", "")
}

func WaitForStartOrStop(ctx context.Context, appName string, machine *fly.Machine, action string, timeout time.Duration) error {
	_, err := waitForStartOrStop(ctx, appName, machine, action, timeout, "", "")
	return err
}

func waitForStartOrStop(ctx context.Context, appName string, machine *fly.Machine, action string, timeout time.Duration, fromEventID, version string) (string, error) {
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
		return "", invalidAction
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
	if version != "" {
		waitOpts = append(waitOpts, flaps.WithWaitVersion(version))
	}
	waitingForFirstStart := false
	for {
		err := waitCtx.Err()
		if err == nil {
			err = flapsClient.Wait(waitCtx, appName, machine.ID, waitOpts...)
		}
		if err == nil {
			return waitOnAction, nil
		}

		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return "", err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return "", WaitTimeoutErr{
				machineID:    machine.ID,
				timeout:      timeout,
				desiredState: waitOnAction,
			}
		default:
			var flapsErr *flaps.FlapsError
			isFlapsError := errors.As(err, &flapsErr)
			if version != "" && isTerminalVersionWaitError(err) {
				return "", err
			}
			terminalNotFound := waitingForFirstStart && errors.Is(err, flaps.ErrFlapsNotFound)
			if fromEventID != "" && machine.Config != nil && machine.Config.AutoDestroy && isFlapsError && flapsErr.ResponseStatusCode == http.StatusNotFound {
				current, getErr := flapsClient.Get(waitCtx, appName, machine.ID)
				var readErr *flaps.FlapsError
				switch {
				case errors.Is(getErr, flaps.ErrFlapsNotFound):
					// An unpinned launch may not be visible yet. Preserve the pinned wait's 404.
				case errors.As(getErr, &readErr) && readErr.ResponseStatusCode >= 400 && readErr.ResponseStatusCode < 500 &&
					readErr.ResponseStatusCode != http.StatusRequestTimeout && readErr.ResponseStatusCode != http.StatusTooManyRequests:
					return "", getErr
				case getErr != nil:
					terminalNotFound = false
				case current != nil && current.ID == machine.ID && current.State == fly.MachineStateDestroying && current.Version == machine.Version:
					terminalNotFound = false
				case current != nil && current.ID == machine.ID && current.State == fly.MachineStateDestroyed:
					if confirmedLaunchStart(current, machine.Version, fromEventID) {
						return fly.MachineStateDestroyed, nil
					}

					return "", fmt.Errorf("Machine %s was destroyed before flyctl could confirm it started", machine.ID)
				}
			}
			if terminalNotFound || (waitingForFirstStart && isFlapsError && flapsErr.ResponseStatusCode == http.StatusConflict) {
				return "", err
			}
			// A settled wait accepts stopped and failed, so a poll timeout only means the version is still in transition.
			if action != "settled" && strings.Contains(err.Error(), "machine failed to reach desired state") && machine.Config != nil && machine.Config.Restart != nil && machine.Config.Restart.Policy == fly.MachineRestartPolicyNo {
				if action != "start" || machine.Version == "" || !isFlapsError || flapsErr.ResponseStatusCode != http.StatusRequestTimeout ||
					flapsErr.Error() != "deadline_exceeded: machine failed to reach desired state, started, currently created" {
					return "", fmt.Errorf("machine failed to reach desired start state, and restart policy was set to %s restart", machine.Config.Restart.Policy)
				}
				// Follow the original launch if another version replaces this one.
				if !waitingForFirstStart {
					waitOpts = append(waitOpts, flaps.WithWaitVersion(machine.Version))
					waitingForFirstStart = true
				}
			}
			if isFlapsError && flapsErr.ResponseStatusCode == http.StatusBadRequest {
				return "", fmt.Errorf("failed waiting for machine: %w", err)
			}
			select {
			case <-waitCtx.Done():
				// Let the existing cancellation and timeout paths report the outcome.
			case <-time.After(b.Duration()):
			}
		}
	}
}

// A launch cursor and start event are ordered ULIDs in the same Machine version.
func confirmedLaunchStart(current *fly.Machine, version, fromEventID string) bool {
	cursor, err := ulid.ParseStrict(fromEventID)
	if err != nil || version == "" || current.Version != version {
		return false
	}
	for _, event := range current.Events {
		if event == nil || event.Type != "start" || event.Status != fly.MachineStateStarted {
			continue
		}
		id, err := ulid.ParseStrict(event.ID)
		if err == nil && id.Compare(cursor) > 0 {
			return true
		}
	}

	return false
}

func isTerminalVersionWaitError(err error) bool {
	var flapsErr *flaps.FlapsError
	return errors.As(err, &flapsErr) &&
		(flapsErr.ResponseStatusCode == http.StatusNotFound || flapsErr.ResponseStatusCode == http.StatusConflict)
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
