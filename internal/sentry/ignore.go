package sentry

import (
	"errors"
	"strings"

	"github.com/superfly/flyctl/internal/flyerr"
	"golang.org/x/crypto/ssh"
)

// ignoredMessages are substrings of expected errors that flyctl does not
// create itself, so they cannot be marked with flyerr.Expected: signal causes
// from the Go runtime and account states reported by the API. Errors flyctl
// creates should use flyerr.Expected instead of being listed here.
var ignoredMessages = []string{
	// Ctrl-C or SIGTERM, surfaced through context.Cause by signal.NotifyContext.
	"signal received",
	// Account states the API already explains to the user.
	"reached its machine limit",
	"has overdue invoices",
}

// ignore reports whether err is an expected outcome that should not be sent
// to Sentry.
func ignore(err error) bool {
	if err == nil {
		return true
	}

	// Walk the wrappers one at a time rather than with errors.As, which also
	// looks inside a combined error and would let one expected part speak for
	// the rest.
	for e := err; e != nil; e = errors.Unwrap(e) {
		if _, ok := e.(interface{ ExpectedOutcome() }); ok {
			return true
		}

		// A remote command exiting non-zero is its result, not a client bug.
		// ExitMissingError is still reported: a channel closing without an
		// exit status usually means the tunnel, agent or hallpass dropped it.
		if _, ok := e.(*ssh.ExitError); ok {
			return true
		}

		if parts := multi(e); parts != nil {
			return ignoreAll(parts)
		}
	}

	// Only reached for a plain chain. A cancellation inside a combined error
	// is judged as one part of it: deploy pools cancel their siblings when one
	// machine fails, and those cancellations must not hide that failure.
	if flyerr.IsCancelledError(err) {
		return true
	}

	msg := err.Error()
	for _, s := range ignoredMessages {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}

// ignoreAll reports whether a combined error (errors.Join, multierror) is
// noise. Every part must be: one crashing machine must not hide a real
// failure on another. Step labels don't count either way, and a combined
// error with nothing but labels is reported.
func ignoreAll(parts []error) bool {
	noise := false

	for _, p := range parts {
		if p == nil || isStepLabel(p) {
			continue
		}

		if !ignore(p) {
			return false
		}

		noise = true
	}

	return noise
}

func isStepLabel(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if _, ok := e.(interface{ StepLabel() }); ok {
			return true
		}
	}

	return false
}

func multi(err error) []error {
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		return e.Unwrap()
	case interface{ WrappedErrors() []error }:
		return e.WrappedErrors()
	}

	return nil
}
