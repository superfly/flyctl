package machine

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestUpdateSettledReadback(t *testing.T) {
	for _, tc := range []struct {
		name, version, state string
		success              bool
	}{
		{"destroying", "accepted", "destroying", true},
		{"promoted", "accepted", "started", true},
		{"failed", "accepted", "failed", false},
		{"superseded", "newer", "started", false},
		{"unknown", "", "started", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &mock.FlapsClient{
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return nil },
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					return &fly.Machine{InstanceID: tc.version, State: tc.state}, nil
				}}
			ctx := flapsutil.NewContextWithClient(t.Context(), client)
			m, err := waitForUpdate(ctx, "app", &fly.Machine{Version: "previous"}, &fly.Machine{ID: "m", InstanceID: "accepted"}, 3*time.Second)
			if (err == nil) != tc.success || (tc.success && m.State != tc.state) {
				t.Fatalf("machine=%v error=%v", m, err)
			}
		})
	}
}

func TestUpdateUsesCallerDeadline(t *testing.T) {
	for _, phase := range []string{"poll backoff", "replacing readback"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := &mock.FlapsClient{
					WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
						if phase == "poll backoff" {
							return &flaps.FlapsError{ResponseStatusCode: 408, OriginalError: errors.New(replacingPollMessage)}
						}
						return nil
					},
					GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
						return &fly.Machine{InstanceID: "accepted", State: "replacing"}, nil
					},
				}
				target := &fly.Machine{ID: "m", InstanceID: "accepted", Config: &fly.MachineConfig{Restart: &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo}}}
				start := time.Now()
				ctx, cancel := context.WithTimeout(flapsutil.NewContextWithClient(t.Context(), client), 50*time.Millisecond)
				defer cancel()
				_, err := waitForUpdate(ctx, "app", &fly.Machine{Version: "previous"}, target, time.Minute)
				var timeout WaitTimeoutErr
				if !errors.As(err, &timeout) || timeout.machineID != target.ID || timeout.desiredState != "settled" || time.Since(start) != 50*time.Millisecond {
					t.Fatalf("elapsed=%s err=%v", time.Since(start), err)
				}
			})
		})
	}
}
