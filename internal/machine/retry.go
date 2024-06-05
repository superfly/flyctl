package machine

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

var machineRetryBackoff = backoff.ExponentialBackOff{
	InitialInterval:     500 * time.Millisecond,
	RandomizationFactor: 0,
	Multiplier:          2,
	MaxInterval:         5 * time.Second,
	MaxElapsedTime:      0,
	Clock:               backoff.SystemClock,
}

// Retry retries a machine operation a few times before giving up
// This is useful for operations like that can fail only to succeed on another try, like machine creation
func Retry(f func() error) error {
	return backoff.Retry(func() error {
		err := f()
		if err == nil {
			return nil
		}
		// TODO: Filter out retryable errors
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
