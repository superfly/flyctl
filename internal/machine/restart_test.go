package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRestartErrors(t *testing.T) {
	timeout := &flaps.FlapsError{ResponseStatusCode: http.StatusRequestTimeout, OriginalError: errors.New("API timeout"), FlyRequestId: "request-id"}
	conflict := &flaps.FlapsError{ResponseStatusCode: http.StatusPreconditionFailed, OriginalError: errors.New("precondition failed")}

	for _, tc := range []struct {
		name          string
		postErr       error
		want          string
		wantRequestID string
	}{
		{"restart 408", fmt.Errorf("failed to restart VM m1: %w", timeout), "restart request timed out before flyctl could confirm the outcome: failed to restart VM m1: API timeout", "request-id"},
		{"precondition", conflict, "could not stop machine m1: precondition failed", ""},
		{"untyped 408", errors.New("408"), "could not stop machine m1: 408", ""},
		{"client deadline", context.DeadlineExceeded, "could not stop machine m1: context deadline exceeded", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			client := &mock.FlapsClient{
				RestartFunc: func(context.Context, string, fly.RestartMachineInput, string) error {
					posts++
					return tc.postErr
				},
			}
			ios, _, stdout, _ := iostreams.Test()
			ctx := flapsutil.NewContextWithClient(iostreams.NewContext(t.Context(), ios), client)
			err := Restart(ctx, "app", &fly.Machine{ID: "m1"}, &fly.RestartMachineInput{}, "nonce")
			require.EqualError(t, err, tc.want)
			require.Equal(t, 1, posts)
			require.NotContains(t, stdout.String(), "restarted successfully")
			require.ErrorIs(t, err, tc.postErr)
			require.Equal(t, tc.wantRequestID, flaps.GetErrorRequestID(err))
		})
	}
}
