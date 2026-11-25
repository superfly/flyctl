package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestWaitForStartWithoutConfig(t *testing.T) {
	attempts := 0
	client := &mock.FlapsClient{WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
		attempts++
		if attempts == 1 {
			return errors.New("machine failed to reach desired state, started, currently stopped")
		}
		return nil
	}}
	ctx := flapsutil.NewContextWithClient(t.Context(), client)
	if err := WaitForStartOrStop(ctx, "app", &fly.Machine{ID: "m1"}, "start", time.Minute); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("got %d waits, want 2", attempts)
	}
}

func TestWaitForFirstStartWithoutRestart(t *testing.T) {
	for _, tc := range []struct {
		state     string
		status    int
		wantRetry bool
	}{
		{"created", 408, true},
		{"stopped", 408, false},
		{"failed", 408, false},
		{"created", 400, false},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.state, tc.status), func(t *testing.T) {
			calls := 0
			client := &mock.FlapsClient{WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
				calls++
				if calls == 1 {
					return &flaps.FlapsError{ResponseStatusCode: tc.status, OriginalError: fmt.Errorf("deadline_exceeded: machine failed to reach desired state, started, currently %s", tc.state)}
				}
				return nil
			}}
			ctx := flapsutil.NewContextWithClient(t.Context(), client)
			m := &fly.Machine{ID: "m1", Version: "01KB1234567890123456789ABC", Config: &fly.MachineConfig{Restart: &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo}}}
			err := WaitForStartOrStop(ctx, "app", m, "start", time.Minute)
			if tc.wantRetry {
				if err != nil || calls != 2 {
					t.Fatalf("waits=%d, err=%v", calls, err)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("waits=%d, err=%v", calls, err)
			}
		})
	}
}

func TestWaitForFirstStartVersion(t *testing.T) {
	const version = "01KB1234567890123456789ABC"
	for _, tc := range []struct {
		name, version string
		status        int
		message       string
		wantCalls     int32
	}{
		{"started", version, http.StatusOK, "", 2},
		{"replaced", version, http.StatusConflict, "aborted: machine version was replaced", 2},
		{"gone", version, http.StatusNotFound, "not_found: machine not found", 2},
		{"empty version", "", http.StatusOK, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/apps/app/machines/m1/wait" || r.URL.Query().Get("state") != "started" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				if got := r.URL.Query().Get("from_event_id"); got != "01JC1V3CW1VEKNMHX7MJ70DE1D" {
					t.Errorf("launch cursor = %q", got)
				}
				switch calls.Add(1) {
				case 1:
					if r.URL.Query().Has("version") {
						t.Errorf("first wait has a version: %s", r.URL)
					}
					w.WriteHeader(http.StatusRequestTimeout)
					fmt.Fprint(w, `{"error":"deadline_exceeded: machine failed to reach desired state, started, currently created"}`)
				case 2:
					if got := r.URL.Query().Get("version"); got != version {
						t.Errorf("second wait version = %q, want %q", got, version)
					}
					w.WriteHeader(tc.status)
					fmt.Fprintf(w, `{"error":%q}`, tc.message)
				default:
					t.Errorf("unexpected extra wait: %s", r.URL)
				}
			}))
			defer server.Close()
			t.Setenv("FLY_FLAPS_BASE_URL", server.URL)
			client, err := flaps.NewWithOptions(t.Context(), flaps.NewClientOpts{Tokens: tokens.Parse("FlyV1 fake")})
			if err != nil {
				t.Fatal(err)
			}
			ctx := flapsutil.NewContextWithClient(t.Context(), &mock.FlapsClient{WaitFunc: client.Wait})
			m := &fly.Machine{ID: "m1", Version: tc.version, Config: &fly.MachineConfig{Restart: &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo}}}
			m.Events = []*fly.MachineEvent{{ID: "01JC1V3CW1VEKNMHX7MJ70DE1D", Type: "launch", Status: "created"}}
			_, err = WaitForLaunch(ctx, "app", m, 5*time.Second)
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("wait calls = %d, want %d", got, tc.wantCalls)
			}
			switch {
			case tc.version == "":
				if err == nil || !strings.Contains(err.Error(), "restart policy was set to no restart") {
					t.Errorf("expected restart policy error, got %v", err)
				}
			case tc.status != http.StatusOK:
				var apiErr *flaps.FlapsError
				if !errors.As(err, &apiErr) || apiErr.ResponseStatusCode != tc.status {
					t.Errorf("expected HTTP %d, got %v", tc.status, err)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}

func TestWaitForAutoDestroyedLaunch(t *testing.T) {
	const cursor = "01M88HFCDT61KY94NYM9HWM0A4"
	start := &fly.MachineEvent{ID: "01M88HFHQ2JRD6X01GVKNZWZBM", Type: "start", Status: "started"}
	missing := &flaps.FlapsError{ResponseStatusCode: http.StatusNotFound, OriginalError: errors.New("machine not found")}
	for _, tc := range []struct {
		name      string
		current   *fly.Machine
		wantState string
		wantWaits int
		wantErr   string
	}{
		{"confirmed", &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{start}}, "destroyed", 1, ""},
		{"never started", &fly.Machine{ID: "m", Version: "v", State: "destroyed"}, "", 1, "before flyctl could confirm it started"},
		{"replaced", &fly.Machine{ID: "m", Version: "other", State: "destroyed", Events: []*fly.MachineEvent{start}}, "", 1, "before flyctl could confirm it started"},
		{"earlier start", &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{{ID: "01M88HFCDDGNKKGXM6KR6JT3V2", Type: "start", Status: "started"}}}, "", 1, "before flyctl could confirm it started"},
		{"malformed event ID", &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{{ID: "zzzz", Type: "start", Status: "started"}}}, "", 1, "before flyctl could confirm it started"},
		{"inherited status", &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{{ID: start.ID, Type: "update", Status: "started"}}}, "", 1, "before flyctl could confirm it started"},
		{"start requested", &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{{ID: start.ID, Type: "start", Status: "starting"}}}, "", 1, "before flyctl could confirm it started"},
		{"missing record", nil, "started", 2, ""},
		{"still creating", &fly.Machine{ID: "m", Version: "v", State: "created"}, "started", 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waits, gets := 0, 0
			client := &mock.FlapsClient{
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
					waits++
					if waits == 1 {
						return missing
					}
					return nil
				},
				GetFunc: func(_ context.Context, app, id string) (*fly.Machine, error) {
					gets++
					if app != "app" || id != "m" {
						t.Fatalf("unexpected lookup %s/%s", app, id)
					}
					if tc.current == nil {
						return nil, missing
					}

					return tc.current, nil
				},
			}
			ctx := flapsutil.NewContextWithClient(t.Context(), client)
			m := &fly.Machine{ID: "m", Version: "v", Config: &fly.MachineConfig{AutoDestroy: true}, Events: []*fly.MachineEvent{{ID: cursor, Type: "launch", Status: "created"}}}
			state, err := WaitForLaunch(ctx, "app", m, 5*time.Second)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if waits != tc.wantWaits || gets != 1 || state != tc.wantState {
				t.Fatalf("state=%q err=%v waits=%d gets=%d", state, err, waits, gets)
			}
		})
	}
}

func TestPinnedAutoDestroyReadback(t *testing.T) {
	const cursor = "01M88HFCDT61KY94NYM9HWM0A4"
	missing := flaps.ErrFlapsNotFound
	for _, tc := range []struct {
		name      string
		readErr   error
		wantRetry bool
	}{
		{"network error", io.ErrUnexpectedEOF, true},
		{"request timeout", &flaps.FlapsError{ResponseStatusCode: http.StatusRequestTimeout}, true},
		{"rate limited", &flaps.FlapsError{ResponseStatusCode: http.StatusTooManyRequests}, true},
		{"unavailable", &flaps.FlapsError{ResponseStatusCode: http.StatusServiceUnavailable}, true},
		{"missing", &flaps.FlapsError{ResponseStatusCode: http.StatusNotFound}, false},
		{"forbidden", &flaps.FlapsError{ResponseStatusCode: http.StatusForbidden}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waits, gets := 0, 0
			client := &mock.FlapsClient{
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
					waits++
					if waits == 1 {
						return &flaps.FlapsError{ResponseStatusCode: http.StatusRequestTimeout, OriginalError: errors.New("deadline_exceeded: machine failed to reach desired state, started, currently created")}
					}
					return missing
				},
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					gets++
					if gets == 1 {
						return nil, tc.readErr
					}
					return &fly.Machine{ID: "m", Version: "v", State: "destroyed", Events: []*fly.MachineEvent{{ID: "01M88HFHQ2JRD6X01GVKNZWZBM", Type: "start", Status: "started"}}}, nil
				},
			}
			ctx := flapsutil.NewContextWithClient(t.Context(), client)
			m := &fly.Machine{ID: "m", Version: "v", Config: &fly.MachineConfig{AutoDestroy: true, Restart: &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo}}, Events: []*fly.MachineEvent{{ID: cursor, Type: "launch", Status: "created"}}}
			state, err := WaitForLaunch(ctx, "app", m, 5*time.Second)
			if tc.wantRetry {
				if err != nil || state != "destroyed" || gets != 2 {
					t.Fatalf("state=%q gets=%d err=%v", state, gets, err)
				}
			} else if !errors.Is(err, missing) || gets != 1 {
				t.Fatalf("gets=%d err=%v", gets, err)
			}
		})
	}
}
