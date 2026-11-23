package machine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
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
