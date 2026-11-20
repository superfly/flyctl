package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestMayAlreadyBeStarting(t *testing.T) {
	const created = "unable to start machine from current state: 'created'"
	for _, tc := range []struct {
		status  int
		message string
		want    bool
	}{
		{http.StatusPreconditionFailed, "failed_precondition: " + created, true},
		{http.StatusPreconditionFailed, created, true},
		{http.StatusConflict, created, false},
		{http.StatusPreconditionFailed, "unable to start machine from current state: 'stopping'", false},
		{http.StatusPreconditionFailed, "machine getting replaced, refusing to start", false},
		{http.StatusConflict, "aborted: machine still attempting to start", true},
		{http.StatusConflict, "machine still attempting to start", true},
	} {
		err := fmt.Errorf("start: %w", &flaps.FlapsError{ResponseStatusCode: tc.status, OriginalError: errors.New(tc.message)})
		if got := startRefusedWhileCreated(err) || startAlreadyInProgress(err); got != tc.want {
			t.Errorf("status %d, message %q: got %v, want %v", tc.status, tc.message, got, tc.want)
		}
	}
}

func TestStartAfterCreation(t *testing.T) {
	original := &fly.Machine{ID: "machine", Version: "v1", LeaseNonce: "original"}
	for _, tc := range []struct {
		name, state, version, nonce         string
		wantConfirmed, wantRetry, wantError bool
	}{
		{"launching create", "started", "v1", "original", true, false, false},
		{"skip launch", "stopped", "v1", "original", false, true, false},
		{"replacement", "stopped", "v2", "original", false, false, true},
		{"different lease", "stopped", "v1", "other", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			starts := 0
			client := &mock.FlapsClient{
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					return &fly.Machine{State: tc.state, Version: tc.version}, nil
				},
				FindLeaseFunc: func(context.Context, string, string) (*fly.MachineLease, error) {
					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: tc.nonce, Version: tc.version, ExpiresAt: time.Now().Add(time.Minute).Unix()}}, nil
				},
				StartFunc: func(_ context.Context, app, id, nonce string) (*fly.MachineStartResponse, error) {
					if app != "app" || id != original.ID || nonce != original.LeaseNonce {
						t.Fatalf("unexpected start target or lease")
					}
					starts++
					return &fly.MachineStartResponse{}, nil
				},
			}
			confirmed, err := startAfterCreation(flapsutil.NewContextWithClient(t.Context(), client), "app", original)
			if confirmed != tc.wantConfirmed || (err != nil) != tc.wantError || (starts == 1) != tc.wantRetry || starts > 1 {
				t.Fatalf("confirmed=%v starts=%d err=%v", confirmed, starts, err)
			}
		})
	}
}
