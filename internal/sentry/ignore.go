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

	// A combined error (errors.Join, multierror) is only noise if every part
	// is; one crashing machine must not hide a real failure on another.
	for e := err; e != nil; e = errors.Unwrap(e) {
		if parts := multi(e); parts != nil {
			for _, p := range parts {
				if !ignore(p) {
					return false
				}
			}
			return true
		}
	}

	if flyerr.IsCancelledError(err) || flyerr.IsExpected(err) {
		return true
	}

	// A remote command exiting non-zero is its result, not a client bug.
	// ExitMissingError is still reported: a channel closing without an exit
	// status usually means the tunnel, agent or hallpass dropped it.
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
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

func multi(err error) []error {
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		return e.Unwrap()
	case interface{ WrappedErrors() []error }:
		return e.WrappedErrors()
	}
	return nil
}
