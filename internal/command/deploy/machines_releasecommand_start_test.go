package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

type releaseStartTransport func(*http.Request) (*http.Response, error)

func (f releaseStartTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStartReleaseCommandMachine(t *testing.T) {
	const notReady = "failed_precondition: machine build not ready, retry the start request"
	for _, tc := range []struct {
		name      string
		status    int
		message   string
		refusals  int
		wantCalls int
		wantError bool
	}{
		{name: "already ready", status: 412, message: notReady, wantCalls: 1},
		{name: "delayed build", status: 412, message: notReady, refusals: 3, wantCalls: 4},
		{name: "unrelated precondition", status: 412, message: "failed_precondition: machine still active, refusing to start", refusals: 1, wantCalls: 1, wantError: true},
		{name: "lease conflict", status: 409, message: "lease currently held", refusals: 1, wantCalls: 1, wantError: true},
		{name: "destroyed", status: 404, message: "machine not found", refusals: 1, wantCalls: 1, wantError: true},
		{name: "ambiguous server error", status: 500, message: notReady, refusals: 1, wantCalls: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				client, err := flaps.NewWithOptions(t.Context(), flaps.NewClientOpts{Transport: releaseStartTransport(func(r *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "/v1/apps/test-app/machines/release-fixture/start", r.URL.Path)
					calls++
					status, body := 200, "{}"
					if calls <= tc.refusals {
						status = tc.status
						body = fmt.Sprintf(`{"error":%q}`, tc.message)
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})})
				require.NoError(t, err)
				ios, _, _, _ := iostreams.Test()
				lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "release-fixture", State: "stopped"}, false)
				err = startReleaseCommandMachine(t.Context(), lm.Start, 10*time.Second)
				if tc.wantError {
					require.ErrorContains(t, err, tc.message)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, tc.wantCalls, calls)
			})
		})
	}
}

func TestStartReleaseCommandMachineContext(t *testing.T) {
	for _, cancelAfter := range []time.Duration{0, 100 * time.Millisecond, 10 * time.Second} {
		t.Run(cancelAfter.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if cancelAfter == 0 {
					cancel()
				} else if cancelAfter < time.Second {
					go func() { time.Sleep(cancelAfter); cancel() }()
				}
				calls := 0
				err := startReleaseCommandMachine(ctx, func(context.Context) error {
					calls++
					return &flaps.FlapsError{ResponseStatusCode: 412, OriginalError: errors.New("failed_precondition: machine build not ready, retry the start request")}
				}, 2500*time.Millisecond)
				if cancelAfter < time.Second {
					require.ErrorIs(t, err, context.Canceled)
					if cancelAfter == 0 {
						require.Zero(t, calls)
					} else {
						require.Equal(t, 1, calls)
					}
				} else {
					require.ErrorContains(t, err, "machine build still not ready after 2.5s (--wait-timeout)")
					require.False(t, errors.Is(err, context.DeadlineExceeded))
					require.Equal(t, 3, calls)
				}
			})
		})
	}
}

func TestStartReleaseCommandMachineInFlightBudget(t *testing.T) {
	for _, finalErr := range []error{nil, &flaps.FlapsError{ResponseStatusCode: 409, OriginalError: errors.New("lease currently held")}} {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			err := startReleaseCommandMachine(t.Context(), func(ctx context.Context) error {
				calls++
				time.Sleep(2 * time.Second)
				require.NoError(t, ctx.Err(), "retry budget must not cancel an in-flight Start")
				return finalErr
			}, time.Second)
			require.ErrorIs(t, err, finalErr)
			require.Equal(t, 1, calls)
		})
	}
}

func TestStartReleaseCommandMachineZeroTimeout(t *testing.T) {
	for _, readyAfter := range []int{0, 2, -1} {
		t.Run(fmt.Sprintf("ready_after_%d", readyAfter), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				began := time.Now()
				calls := 0
				err := startReleaseCommandMachine(t.Context(), func(ctx context.Context) error {
					require.NoError(t, ctx.Err())
					calls++
					if readyAfter >= 0 && calls > readyAfter {
						return nil
					}
					return &flaps.FlapsError{ResponseStatusCode: http.StatusPreconditionFailed, OriginalError: errors.New("failed_precondition: machine build not ready, retry the start request")}
				}, 0)
				if readyAfter >= 0 {
					require.NoError(t, err)
					require.Equal(t, readyAfter+1, calls)
				} else {
					require.ErrorContains(t, err, "machine build still not ready after 2m0s (--wait-timeout)")
					require.Equal(t, 2*time.Minute, time.Since(began))
					require.Greater(t, calls, 1, "zero timeout must allow retries before the fallback expires")
				}
			})
		})
	}
}

func TestStartReleaseCommandMachinePreservesRefusal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		refusal := &flaps.FlapsError{ResponseStatusCode: 412, OriginalError: errors.New("failed_precondition: machine build not ready, retry the start request"), FlyRequestId: "request-fixture", TraceID: "0123456789abcdef0123456789abcdef"}
		original := fmt.Errorf("failed to start VM fixture: %w", refusal)
		err := startReleaseCommandMachine(t.Context(), func(context.Context) error { return original }, 500*time.Millisecond)
		require.ErrorIs(t, err, original)
		require.Equal(t, refusal.FlyRequestId, flaps.GetErrorRequestID(err))
		require.Equal(t, refusal.TraceID, flaps.GetErrorTraceID(err))
		require.ErrorContains(t, err, "--wait-timeout")
		require.False(t, errors.Is(err, context.DeadlineExceeded))
	})
}

func TestStartReleaseCommandMachineCallerCancelsInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { time.Sleep(100 * time.Millisecond); cancel() }()
		calls := 0
		err := startReleaseCommandMachine(ctx, func(ctx context.Context) error { calls++; <-ctx.Done(); return ctx.Err() }, time.Minute)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, calls)
	})
}
