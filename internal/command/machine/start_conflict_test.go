package machine

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/superfly/fly-go/flaps"
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
		if got := mayAlreadyBeStarting(err); got != tc.want {
			t.Errorf("status %d, message %q: got %v, want %v", tc.status, tc.message, got, tc.want)
		}
	}
}
