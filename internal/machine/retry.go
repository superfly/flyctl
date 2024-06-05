package machine

import (
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func isRetryable(err error) bool {

	if strings.Contains(err.Error(), "request returned non-2xx status, 504") {
		return true
	}
	return false
}

// Retry retries a machine operation a few times before giving up
// This is useful for operations like that can fail only to succeed on another try, like machine creation
// timeout does not cancel operations proactively, it is only passed to the operation to allow it to cancel itself
// this is useful for operations that have a timeout, like machine creation.
// If an operation fails, subsequent calls to the operation will have their timeout shortened by the already elapsed time
func Retry(timeout time.Duration, f func(timeout time.Duration) error) error {

	var machineRetryBackoff = backoff.ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0,
		Multiplier:          2,
		MaxInterval:         5 * time.Second,
		MaxElapsedTime:      0,
		Clock:               backoff.SystemClock,
	}

	end := time.Now().Add(timeout)
	var lastError error

	return backoff.Retry(func() error {
		var remainingTime time.Duration
		if timeout != 0 {
			// We have to handle all this for edge cases anyway, so we'll just handle exiting due to timeout
			// in here *exclusively* instead of relying on machineRetryBackoff.MaxElapsedTime
			remainingTime = time.Until(end)
			if remainingTime <= 0 {
				if lastError != nil {
					// We don't have enough allotted time to run the operation again, and we *were*
					// provided a timeout, so we should just return the last error
					return backoff.Permanent(lastError)
				} else {
					// We get here if
					//  * there IS a timeout
					//  * there's no previous error (meaning this is iteration 1)
					//  * the timeout is already expired
					// This should never happen, but just in case, we'll just run this iteration with the full timeout
					// since we *know* it's the first time around anyway
					remainingTime = timeout
				}
			}
		}
		err := f(remainingTime)
		if err == nil {
			return nil
		}
		lastError = err

		if isRetryable(err) {
			return err
		}
		return backoff.Permanent(err)
	}, &machineRetryBackoff)
}

// RetryRet retries a machine operation a few times before giving up
// This is useful for operations like that can fail only to succeed on another try, like machine creation
func RetryRet[T any](timeout time.Duration, f func(time.Duration) (T, error)) (T, error) {
	var res T
	err := Retry(timeout, func(localTimeout time.Duration) error {
		var err error
		res, err = f(localTimeout)
		return err
	})
	return res, err
}
