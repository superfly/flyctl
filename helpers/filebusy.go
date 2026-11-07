package helpers

import (
	"time"

	"github.com/avast/retry-go/v4"
)

// fileBusyAttempts and fileBusyDelay bound RetryIfFileBusy to about a quarter
// of a second: long enough to outlast another process's read, short enough
// that a file held open for good fails promptly.
const (
	fileBusyAttempts = 10
	fileBusyDelay    = 25 * time.Millisecond
)

// RetryIfFileBusy runs fn, an operation on the file at path, running it again
// for a short while if it fails because another process has the file open.
// Windows refuses to rename over or delete a file while it is open elsewhere,
// and refuses to open one while a rename over it is in progress; elsewhere fn
// runs once.
func RetryIfFileBusy(path string, fn func() error) error {
	return retryWhile(fn, func(err error) bool { return IsFileBusy(path, err) }, fileBusyAttempts, fileBusyDelay)
}

// retryWhile runs fn up to attempts times, pausing delay between runs, for as
// long as its error satisfies retryable, and returns its last error.
func retryWhile(fn func() error, retryable func(error) bool, attempts uint, delay time.Duration) error {
	return retry.Do(fn,
		retry.RetryIf(retryable),
		retry.Attempts(attempts),
		retry.Delay(delay),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
	)
}
