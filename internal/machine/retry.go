package machine

import (
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

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

		if strings.Contains(err.Error(), "request returned non-2xx status, 504") {
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
