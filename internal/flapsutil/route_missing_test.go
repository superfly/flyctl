package flapsutil

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/flaps"
)

func TestIsFlapsRouteMissing(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "plain text 404 (unmatched route)",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("request returned non-2xx status: 404: 404 page not found\n"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte("404 page not found\n"),
			},
			want: true,
		},
		{
			name: "JSON resource 404 with error field",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("database not found"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte(`{"error":"database not found"}`),
			},
			want: false,
		},
		{
			// ui-ex renders an unmatched route (and, on older builds, an
			// authz 404) as its default ErrorJSON body. Keep falling
			// back rather than guess which one it was.
			name: "ui-ex default JSON 404 body",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("request returned non-2xx status: 404"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte(`{"errors":{"detail":"not found"}}`),
			},
			want: true,
		},
		{
			name: "JSON resource 404 with message field",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("User not found"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte(`{"message":"User not found"}`),
			},
			want: false,
		},
		{
			name: "empty body 404",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("request returned non-2xx status: 404: "),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte{},
			},
			want: true,
		},
		{
			name: "nil body 404",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("request returned non-2xx status: 404: "),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       nil,
			},
			want: true,
		},
		{
			name: "JSON object with neither error nor message field",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("not found"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte(`{"status":"unknown"}`),
			},
			want: true,
		},
		{
			name: "non-404 status",
			err: &flaps.FlapsError{
				OriginalError:      errors.New("internal error"),
				ResponseStatusCode: http.StatusInternalServerError,
				ResponseBody:       []byte(`{"error":"boom"}`),
			},
			want: false,
		},
		{
			name: "non-FlapsError",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "wrapped FlapsError still detected",
			err: fmt.Errorf("failed retrieving cluster: %w", &flaps.FlapsError{
				OriginalError:      errors.New("request returned non-2xx status: 404: 404 page not found\n"),
				ResponseStatusCode: http.StatusNotFound,
				ResponseBody:       []byte("404 page not found\n"),
			}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsFlapsRouteMissing(tt.err))
		})
	}
}
