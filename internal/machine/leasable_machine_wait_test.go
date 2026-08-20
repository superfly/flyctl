package machine

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/iostreams"
)

type waitRoundTripFunc func(*http.Request) (*http.Response, error)

func (f waitRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWaitForStatePinsInstanceIDAsVersion(t *testing.T) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	const instanceID = "01G6R2TQGS41MBQTCA55X8ZCZW"
	var gotVersion string
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: waitRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotVersion = req.URL.Query().Get("version")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
	})
	require.NoError(t, err)

	ios, _, _, _ := iostreams.Test()
	lm := NewLeasableMachine(client, ios, "app", &fly.Machine{
		ID:         "machine-id",
		InstanceID: instanceID,
	}, false)

	err = lm.WaitForState(context.Background(), fly.MachineStateStopped, time.Second, WithVersion(lm.Machine().InstanceID))
	require.NoError(t, err)
	require.Equal(t, instanceID, gotVersion)
}
