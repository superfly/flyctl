package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
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

type startTransport func(*http.Request) (*http.Response, error)

func (f startTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRunMachineStartDuringCreation(t *testing.T) {
	for _, tc := range []struct {
		settledState string
		wantStarts   int
	}{
		{"started", 1},
		{"stopped", 2},
	} {
		t.Run(tc.settledState, func(t *testing.T) {
			const path = "/v1/apps/test-app/machines/test-machine"
			state := "created"
			starts, waits := 0, 0
			client, err := flaps.NewWithOptions(t.Context(), flaps.NewClientOpts{Transport: startTransport(func(r *http.Request) (*http.Response, error) {
				status, body := http.StatusOK, "{}"
				switch r.Method + " " + r.URL.Path {
				case "GET " + path:
					body = fmt.Sprintf(`{"id":"test-machine","instance_id":"v1","version":"v1","host_status":"ok","state":%q}`, state)
				case "POST " + path + "/lease", "GET " + path + "/lease":
					body = fmt.Sprintf(`{"data":{"nonce":"held","version":"v1","expires_at":%d}}`, time.Now().Add(time.Minute).Unix())
				case "POST " + path + "/start":
					starts++
					require.Equal(t, []string{"held"}, r.Header[flaps.NonceHeader])
					if starts == 1 {
						require.Equal(t, "created", state)
						status = http.StatusPreconditionFailed
						body = `{"error":"failed_precondition: unable to start machine from current state: 'created'"}`
					} else {
						require.Equal(t, "stopped", state)
						state = "started"
					}
				case "GET " + path + "/wait":
					require.Equal(t, 1, starts)
					require.Equal(t, "v1", r.URL.Query().Get("version"))
					require.ElementsMatch(t, []string{"started", "stopped"}, r.URL.Query()["state"])
					waits++
					state = tc.settledState
				case "DELETE " + path + "/lease":
				default:
					return nil, fmt.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}

				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})})
			require.NoError(t, err)
			cmd := newStart()
			require.NoError(t, cmd.ParseFlags([]string{"test-machine"}))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = appconfig.WithName(ctx, "test-app")
			require.NoError(t, runMachineStart(flapsutil.NewContextWithClient(ctx, client)))
			require.Equal(t, 1, waits)
			require.Equal(t, tc.wantStarts, starts)
			require.Contains(t, out.String(), "test-machine has been started")
		})
	}
}
