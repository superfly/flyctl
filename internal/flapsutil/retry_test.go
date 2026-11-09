package flapsutil

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/flaps"
)

func TestIsTransientFlapsError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil is not retryable", err: nil, want: false},
		{name: "context canceled is not retryable", err: context.Canceled, want: false},
		{name: "context deadline exceeded is not retryable", err: context.DeadlineExceeded, want: false},
		{
			name: "flaps 408 is retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusRequestTimeout, OriginalError: errors.New("upstream timeout")},
			want: true,
		},
		{
			name: "flaps 429 is retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusTooManyRequests, OriginalError: errors.New("rate limited")},
			want: true,
		},
		{
			name: "flaps 502 is retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusBadGateway, OriginalError: errors.New("bad gateway")},
			want: true,
		},
		{
			name: "flaps 503 is retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusServiceUnavailable, OriginalError: errors.New("unavailable")},
			want: true,
		},
		{
			name: "flaps 400 is not retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusBadRequest, OriginalError: errors.New("bad request")},
			want: false,
		},
		{
			name: "flaps 404 is not retryable",
			err:  &flaps.FlapsError{ResponseStatusCode: http.StatusNotFound, OriginalError: errors.New("not found")},
			want: false,
		},
		{
			name: "connection reset is retryable",
			err:  errors.New("read tcp 1.2.3.4:443: connection reset by peer"),
			want: true,
		},
		{
			name: "connection refused is retryable",
			err:  errors.New("dial tcp: connection refused"),
			want: true,
		},
		{
			name: "no such host is retryable",
			err:  errors.New("lookup foo.internal: no such host"),
			want: true,
		},
		{
			name: "unrelated error is not retryable",
			err:  errors.New("machine is misconfigured"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsTransientFlapsError(tc.err))
		})
	}
}

func TestHasTransientNetworkSubstring(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil returns false", err: nil, want: false},
		{name: "connection reset matches", err: errors.New("Connection reset by peer"), want: true},
		{name: "connection refused matches", err: errors.New("dial: connection refused"), want: true},
		{name: "network unreachable matches", err: errors.New("network is unreachable"), want: true},
		{name: "no such host matches", err: errors.New("lookup: no such host"), want: true},
		{name: "eof matches", err: errors.New("unexpected EOF"), want: true},
		{name: "unrelated does not match", err: errors.New("permission denied"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasTransientNetworkSubstring(tc.err))
		})
	}
}
