package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

func (md *machineDeployment) verifyFastReleaseExit(ctx context.Context, expected *fly.Machine) error {
	if expected == nil || expected.ID == "" || expected.InstanceID == "" {
		return errors.New("missing Machine or version")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	path := "/apps/" + url.PathEscape(md.app.Name) + "/machines/" + url.PathEscape(expected.ID)
	req, err := md.flapsClient.NewRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Machine lookup returned %s", resp.Status)
	}
	var receipt struct {
		ID         string `json:"id"`
		InstanceID string `json:"instance_id"`
		Version    string `json:"version"`
		State      string `json:"state"`
		Events     []struct {
			ID, Type, Status string
			Request          struct {
				Exit    *releaseExitReceipt `json:"exit_event"`
				Monitor *struct {
					Exit *releaseExitReceipt `json:"exit_event"`
				} `json:"MonitorEvent"`
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&receipt); err != nil {
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
	var exit *releaseExitReceipt
	for i, event := range receipt.Events {
		switch event.Type {
		case "destroy":
			continue
		case "exit":
			if exit != nil || event.ID == "" || event.Status != "stopped" {
				return errors.New("ambiguous exit event")
			}
			exit = event.Request.Exit
			if event.Request.Monitor != nil {
				if exit != nil {
					return errors.New("ambiguous exit payload")
				}
				exit = event.Request.Monitor.Exit
			}
			if exit == nil || exit.ExitCode == nil || *exit.ExitCode != 0 || (exit.GuestExitCode != nil && *exit.GuestExitCode != 0) || exit.Signal > 0 || exit.GuestSignal > 0 || exit.Error != "" || exit.GuestError != "" || exit.OOMKilled || exit.Restarting || exit.RequestedStop || exit.ExitedAt.UnixMilli() <= 0 {
				return errors.New("no successful exit record")
			}
		case "start":
			if exit != nil && event.ID != "" && (event.Status == "started" || event.Status == "starting") {
				if len(expected.Events) > 0 {
					anchor := expected.Events[0]
					if anchor == nil || anchor.ID == "" {
						return errors.New("missing pre-start event identity")
					}
					for _, older := range receipt.Events[i+1:] {
						if older.ID == anchor.ID && older.Type == anchor.Type && older.Status == anchor.Status {
							return ctx.Err()
						}
						if older.Type != "start" || older.Status != "starting" {
							return errors.New("Machine history changed before Start")
						}
					}
					return errors.New("missing pre-start event in Machine history")
				}
				return ctx.Err()
			}
			return errors.New("no exit after latest start")
		default:
			return fmt.Errorf("unexpected Machine event %q", event.Type)
		}
	}
	return errors.New("missing start or exit event")
}
