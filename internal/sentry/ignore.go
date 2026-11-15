package sentry

import (
	"errors"
	"strings"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flyerr"
	"golang.org/x/crypto/ssh"
)

// machineLimitMessage is how flaps says an organization is at its Machine
// limit. flaps gives the refusal no status code of its own.
const machineLimitMessage = "reached its machine limit"

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

		// An account state flaps already explains to the user. Matched on
		// the flaps error itself, so the phrase turning up in some other
		// error's text does not drop that error.
		if fe, ok := e.(*flaps.FlapsError); ok && fe.ResponseStatusCode < 500 && strings.Contains(fe.Error(), machineLimitMessage) {
			return true
		}

		if parts := multi(e); parts != nil {
			return ignoreAll(parts)
		}
	}

	// Only reached for a plain chain. A cancellation inside a combined error
	// is judged as one part of it: deploy pools cancel their siblings when one
	// machine fails, and those cancellations must not hide that failure.
	//
	// This also covers Ctrl-C and SIGTERM: the cause signal.NotifyContext
	// cancels with ("interrupt signal received") reports itself as
	// context.Canceled.
	return flyerr.IsCancelledError(err)
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
