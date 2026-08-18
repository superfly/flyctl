package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/internal/uiex"
)

func TestIsRetryableReleaseStatusError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error is not retried",
			err:  nil,
			want: false,
		},
		{
			// The failure that motivated this retry: HAProxy in front of api.fly.io
			// gives up on the backend and serves its own 504 page.
			name: "504 from the proxy is retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 502, Body: "<html>502 Bad Gateway</html>"},
			want: true,
		},
		{
			name: "gateway timeout is retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 504, Body: "<html>504 Gateway Time-out</html>"},
			want: true,
		},
		{
			name: "500 is retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 500, Body: "boom"},
			want: true,
		},
		{
			name: "wrapped 5xx is still retried",
			err:  fmt.Errorf("updating release: %w", &uiex.StatusError{Op: "update release", StatusCode: 503, Body: "unavailable"}),
			want: true,
		},
		{
			name: "401 is not retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 401, Body: "unauthorized"},
			want: false,
		},
		{
			name: "404 is not retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 404, Body: "not found"},
			want: false,
		},
		{
			name: "422 is not retried",
			err:  &uiex.StatusError{Op: "update release", StatusCode: 422, Body: "the release has already finished"},
			want: false,
		},
		{
			name: "user interrupt is not retried",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "deadline exceeded is not retried",
			err:  context.DeadlineExceeded,
			want: false,
		},
		{
			// No response was ever produced, so there is nothing to inspect a status
			// code on. These are transport-level and worth another attempt.
			name: "connection failure is retried",
			err:  &net.OpError{Op: "dial", Err: errors.New("connection reset by peer")},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetryableReleaseStatusError(tc.err))
		})
	}
}

func TestStatusErrorMessageIsUnchanged(t *testing.T) {
	// The message must keep matching what callers and users saw before StatusError
	// was introduced, so error output and any log scraping stay stable.
	err := &uiex.StatusError{
		Op:         "update release",
		StatusCode: 504,
		Body:       "<html><body><h1>504 Gateway Time-out</h1>\nThe server didn't respond in time.\n</body></html>",
	}

	assert.Equal(t,
		"failed to update release (status 504): <html><body><h1>504 Gateway Time-out</h1>\nThe server didn't respond in time.\n</body></html>",
		err.Error(),
	)
}

func TestStatusErrorRetryable(t *testing.T) {
	assert.False(t, (&uiex.StatusError{StatusCode: 499}).Retryable())
	assert.True(t, (&uiex.StatusError{StatusCode: 500}).Retryable())
	assert.True(t, (&uiex.StatusError{StatusCode: 599}).Retryable())
}
