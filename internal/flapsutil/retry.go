package flapsutil

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/superfly/fly-go/flaps"
)

// IsTransientFlapsError reports whether an error returned by flaps is worth
// another attempt. It combines two signals:
//
//   - HTTP status codes flaps returns for transient upstream conditions:
//     408 Request Timeout (typically a context.DeadlineExceeded that flaps
//     hit talking to flyd), 429 Too Many Requests, and any 5xx.
//   - Transport-level errors from before the request landed (connection
//     reset/refused, EOF, DNS failures).
//
// Context cancellation and deadline expiry are treated as explicit
// instructions to stop, not as transient failures.
//
// IMPORTANT: for non-idempotent endpoints (like flaps' create-machine)
// a 408 from flaps is *ambiguous* — the upstream side-effect may or may
// not have happened. Callers targeting those endpoints should either
// implement client-side idempotency (see the launch-id pattern in the
// bluegreen deploy strategy) or restrict retries to pre-request transport
// errors via HasTransientNetworkSubstring.
func IsTransientFlapsError(err error) bool {
	if err == nil {
		return false
	}

	// User interrupted / deadline elapsed for the whole operation: don't retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var flapsErr *flaps.FlapsError
	if errors.As(err, &flapsErr) {
		switch {
		case flapsErr.ResponseStatusCode == http.StatusRequestTimeout,
			flapsErr.ResponseStatusCode == http.StatusTooManyRequests,
			flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600:
			return true
		}
	}

	return HasTransientNetworkSubstring(err)
}

// HasTransientNetworkSubstring reports whether the error message contains a
// well-known substring associated with transport-level failures that occur
// *before* the request reaches the server. These are always safe to retry,
// even for non-idempotent operations, because no side effect can have
// happened server-side yet.
func HasTransientNetworkSubstring(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, s := range []string{
		"connection reset by peer",
		"connection refused",
		"network is unreachable",
		"temporary failure in name resolution",
		"no such host",
		"eof",
	} {
		if strings.Contains(message, s) {
			return true
		}
	}

	return false
}
