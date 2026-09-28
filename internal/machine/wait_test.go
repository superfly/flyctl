package machine

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

type waitRoundTripFunc func(*http.Request) (*http.Response, error)

func (f waitRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newWaitQueryRecorder returns a flaps client that answers every request with
// 200 and stores the query of the last request in *query.
func newWaitQueryRecorder(t *testing.T, query *url.Values) *flaps.Client {
	t.Helper()
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: waitRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			*query = req.URL.Query()

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Request:    req,
			}, nil
		}),
	})
	require.NoError(t, err)

	return client
}

func TestWaitForStartOrStopSendsVersionOnlyForStop(t *testing.T) {
	var query url.Values
	ctx := flapsutil.NewContextWithClient(context.Background(), newWaitQueryRecorder(t, &query))
	m := &fly.Machine{ID: "machine-id", Version: "01G6R2TQGS41MBQTCA55X8ZCZW"}

	require.NoError(t, WaitForStartOrStop(ctx, "app", m, "stop", time.Second))
	require.Equal(t, fly.MachineStateStopped, query.Get("state"))
	require.Equal(t, m.Version, query.Get("version"))

	require.NoError(t, WaitForStartOrStop(ctx, "app", m, "start", time.Second))
	require.Equal(t, fly.MachineStateStarted, query.Get("state"))
	require.False(t, query.Has("version"))
}

func TestLeasableWaitForStateSendsVersionOnlyForStopped(t *testing.T) {
	var query url.Values
	ios, _, _, _ := iostreams.Test()
	m := &fly.Machine{ID: "machine-id", Version: "01G6R2TQGS41MBQTCA55X8ZCZW"}
	lm := NewLeasableMachine(newWaitQueryRecorder(t, &query), ios, "app", m, false)

	require.NoError(t, lm.WaitForState(context.Background(), fly.MachineStateStopped, time.Second))
	require.Equal(t, fly.MachineStateStopped, query.Get("state"))
	require.Equal(t, m.Version, query.Get("version"))

	require.NoError(t, lm.WaitForState(context.Background(), fly.MachineStateStarted, time.Second))
	require.Equal(t, fly.MachineStateStarted, query.Get("state"))
	require.False(t, query.Has("version"))
}
