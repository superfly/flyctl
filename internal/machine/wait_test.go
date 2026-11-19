package machine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

type waitRoundTripFunc func(*http.Request) (*http.Response, error)

func (f waitRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestWaitForStartOrStopStoppedSendsVersion verifies that waiting for a
// Machine to reach "stopped" sends the Machine's version, which the Machines
// API requires for that state.
func TestWaitForStartOrStopStoppedSendsVersion(t *testing.T) {
	var gotQuery string
	transport := waitRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{Transport: transport})
	require.NoError(t, err)

	ctx := flapsutil.NewContextWithClient(context.Background(), client)
	m := &fly.Machine{ID: "m1", Version: "01HMVXC16ED6SHK9452AF029Y7"}

	err = WaitForStartOrStop(ctx, "test-app", m, "stop", 5*time.Second)
	require.NoError(t, err)

	assert.Contains(t, gotQuery, "state=stopped")
	assert.Contains(t, gotQuery, "version="+m.Version, "wait for stopped state must send the Machine version; query was %q", gotQuery)
}

// TestWaitForStartOrStopStoppedFallsBackToInstanceID verifies that when a
// Machine object has no Version, the wait for "stopped" still sends a version
// (the InstanceID), as the pre-#4778 code did.
func TestWaitForStartOrStopStoppedFallsBackToInstanceID(t *testing.T) {
	var gotQuery string
	transport := waitRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{Transport: transport})
	require.NoError(t, err)

	ctx := flapsutil.NewContextWithClient(context.Background(), client)
	m := &fly.Machine{ID: "m1", InstanceID: "01HMVXC16ED6SHK9452AF029Y8"}

	err = WaitForStartOrStop(ctx, "test-app", m, "stop", 5*time.Second)
	require.NoError(t, err)

	assert.Contains(t, gotQuery, "state=stopped")
	assert.Contains(t, gotQuery, "version="+m.InstanceID, "wait for stopped state must fall back to InstanceID; query was %q", gotQuery)
}

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
