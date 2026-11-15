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

// ExpectedOutcome identifies the mark on this error itself. IsExpected also
// looks inside combined errors; this lets a caller tell the two apart.
func (expectedError) ExpectedOutcome() {}

// StepLabel marks err as the name of the step that failed, joined onto the
// real cause with errors.Join. A label is not a failure of its own: whether
// the combined error is an expected outcome depends on the cause alone. The
// message and errors.Is/As are unchanged.
func StepLabel(err error) error {
	if err == nil {
		return nil
	}

	return labelError{err}
}

type labelError struct{ error }

func (e labelError) Unwrap() error { return e.error }

// StepLabel identifies the mark on this error itself.
func (labelError) StepLabel() {}
