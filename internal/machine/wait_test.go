package machine

import (
	"context"
	"errors"
	"fmt"
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
			err = WaitForStartOrStop(ctx, "app", m, "start", 5*time.Second)
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
