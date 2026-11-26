package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
)

// Carries an already-verified exit through the existing Start retry loop.
var errVerifiedReleaseExit = errors.New("release command already completed successfully")

func (md *machineDeployment) checkReleaseCommandStart(ctx context.Context, expected *fly.Machine, startErr error) error {
	verified, err := md.finishReleaseCommandStart(ctx, expected, startErr)
	if verified {
		return errVerifiedReleaseExit
	}
	return err
}

func (md *machineDeployment) finishReleaseCommandStart(ctx context.Context, expected *fly.Machine, startErr error) (bool, error) {
	var refusal *flaps.FlapsError
	if !errors.As(startErr, &refusal) || refusal.ResponseStatusCode != http.StatusConflict || refusal.Error() != "aborted: machine exited abruptly" {
		return false, startErr
	}
	if err := md.verifyFastReleaseExit(ctx, expected); err != nil {
		return false, fmt.Errorf("%w (could not verify release command exit: %v)", startErr, err)
	}

	return true, nil
}

// The SDK decodes a missing exit_code into an int with value zero. Read this
// exceptional response with a pointer so only an explicit zero is accepted.
type releaseExitReceipt struct {
	ExitCode      *int      `json:"exit_code"`
	GuestExitCode *int      `json:"guest_exit_code"`
	Signal        int       `json:"signal"`
	GuestSignal   int       `json:"guest_signal"`
	OOMKilled     bool      `json:"oom_killed"`
	RequestedStop bool      `json:"requested_stop"`
	Restarting    bool      `json:"restarting"`
	Error         string    `json:"error"`
	GuestError    string    `json:"guest_error"`
	ExitedAt      time.Time `json:"exited_at"`
}

type releaseHistoryEvent struct {
	ID, Type, Status string
	Request          struct {
		Exit    *releaseExitReceipt `json:"exit_event"`
		Monitor *struct {
			Exit *releaseExitReceipt `json:"exit_event"`
		} `json:"MonitorEvent"`
	}
}

func (e releaseHistoryEvent) successfulExit() bool {
	x := e.Request.Exit
	if e.Request.Monitor != nil {
		if x != nil {
			return false
		}
		x = e.Request.Monitor.Exit
	}

	return x != nil && x.ExitCode != nil && *x.ExitCode == 0 && x.ExitedAt.UnixMilli() > 0 &&
		(x.GuestExitCode == nil || *x.GuestExitCode == 0) && x.Signal <= 0 && x.GuestSignal <= 0 &&
		!x.OOMKilled && !x.RequestedStop && !x.Restarting && x.Error == "" && x.GuestError == ""
}

type releaseHistoryReader interface {
	GetMachineVersion(context.Context, string, string, string, any) error
	GetMachineEvents(context.Context, string, string, int, any) error
}

var _ releaseHistoryReader = (*flaps.Client)(nil)

func (md *machineDeployment) verifyFastReleaseExit(ctx context.Context, expected *fly.Machine) error {
	if expected == nil || expected.ID == "" || expected.InstanceID == "" {
		return errors.New("missing Machine or version")
	}
	if len(expected.Events) == 0 || expected.Events[0] == nil || expected.Events[0].ID == "" {
		return errors.New("missing pre-start event identity")
	}
	reader, ok := md.flapsClient.(releaseHistoryReader)
	if !ok {
		return errors.New("client does not support release history reads")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var receipt struct {
		ID             string
		InstanceID     string `json:"instance_id"`
		Version, State string
		Events         []releaseHistoryEvent
	}
	if err := reader.GetMachineVersion(ctx, md.app.Name, expected.ID, expected.InstanceID, &receipt); err != nil {
		return err
	}
	if receipt.ID != expected.ID || receipt.InstanceID != expected.InstanceID || (receipt.Version != "" && receipt.Version != expected.InstanceID) {
		return errors.New("Machine identity or version changed")
	}
	switch receipt.State {
	case "stopped", "destroying", "destroyed":
	default:
		return errors.New("Machine has no terminal result")
	}
	events := receipt.Events
	for len(events) > 0 && events[0].Type == "destroy" {
		events = events[1:]
	}
	if len(events) < 2 || events[0].ID == "" || events[0].Type != "exit" || events[0].Status != "stopped" || !events[0].successfulExit() {
		return errors.New("no verified successful exit")
	}
	start := events[1]
	if start.ID == "" || start.ID == events[0].ID || start.Type != "start" || (start.Status != "starting" && start.Status != "started") {
		return errors.New("exit does not follow Start")
	}
	anchor := expected.Events[0]
	// Destroy can push the pre-start event out of the five events returned by
	// Get. Join the longer history to that versioned snapshot by immutable IDs.
	recent := receipt.Events
	matchesAnchor := func(e releaseHistoryEvent) bool {
		return e.ID == anchor.ID && e.Type == anchor.Type && e.Status == anchor.Status
	}
	if len(recent) == 5 && !slices.ContainsFunc(recent, matchesAnchor) {
		var full []releaseHistoryEvent
		if err := reader.GetMachineEvents(ctx, md.app.Name, expected.ID, 20, &full); err != nil {
			return err
		}
		// Auto-destroy may have prepended these transitions since the versioned
		// read. Keep the entire original snapshot for the comparison below.
		for _, status := range []string{"destroyed", "destroying"} {
			if len(full) > 0 && full[0].ID != "" && full[0].ID != recent[0].ID &&
				full[0].Type == "destroy" && full[0].Status == status {
				full = full[1:]
			}
		}
		if len(full) < len(recent) {
			return errors.New("incomplete Machine history")
		}
		for i, e := range recent {
			if e.ID == "" || full[i].ID != e.ID || full[i].Status != e.Status || full[i].Type != e.Type {
				return errors.New("Machine history changed during verification")
			}
		}
		events = append(events[:len(events):len(events)], full[len(recent):]...)
	}
	// A normal create can return before update/stopped is published. The
	// recovered create already captured that event before sending Start.
	older := events[2:]
	if start.Status == "started" && len(older) > 0 && older[0].Type == "start" && older[0].Status == "starting" {
		older = older[1:]
	}
	for _, transition := range [][2]string{{"update", "stopped"}, {"launch", "created"}} {
		if len(older) == 0 || matchesAnchor(older[0]) {
			break
		}
		if older[0].Type == transition[0] && older[0].Status == transition[1] {
			older = older[1:]
		}
	}
	if len(older) > 0 && matchesAnchor(older[0]) {
		return ctx.Err()
	}

	return errors.New("Machine history does not match the pre-start observation")
}
