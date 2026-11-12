package flyerr

import "errors"

// Expected marks err as an expected outcome of a command, such as the user
// aborting or their own app failing, rather than a flyctl bug. Expected
// errors are not sent to Sentry. The message and the wrapped chain are
// unchanged, so the CLI output and errors.Is/As behave as before.
func Expected(err error) error {
	if err == nil {
		return nil
	}

	return expectedError{err}
}

// IsExpected reports whether err, or anything it wraps, was marked Expected.
func IsExpected(err error) bool {
	var e expectedError

	return errors.As(err, &e)
}

type expectedError struct{ error }

func (e expectedError) Unwrap() error { return e.error }
