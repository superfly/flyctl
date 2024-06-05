package machine

import (
	"errors"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/superfly/fly-go/flaps"
)

func isRetryable(err error) bool {

	var flapsErr *flaps.FlapsError
	if errors.As(err, &flapsErr) {
		switch flapsErr.ResponseStatusCode {
		case http.StatusServiceUnavailable:
			return true
		case http.StatusGatewayTimeout:
			return true
		}
	}

	return false
}

// Retry retries a machine operation a few times before giving up
// This is useful for operations like that can fail only to succeed on another try, like machine creation
func Retry(f func() error) error {

	var machineRetryBackoff = backoff.ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0,
		Multiplier:          2,
		MaxInterval:         5 * time.Second,
		MaxElapsedTime:      0,
		Clock:               backoff.SystemClock,
	}

	return backoff.Retry(func() error {
		err := f()
		if err == nil {
			return nil
		}
		if isRetryable(err) {
			return err
		}
		return backoff.Permanent(err)
	}, &machineRetryBackoff)
}

// RetryRet retries a machine operation a few times before giving up
// This is useful for operations like that can fail only to succeed on another try, like machine creation
func RetryRet[T any](f func() (T, error)) (T, error) {
	var res T
	err := Retry(func() error {
		var err error
		res, err = f()
		return err
	})
	return res, err
}
