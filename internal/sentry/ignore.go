package sentry

import (
	"errors"
	"strings"

	"github.com/superfly/flyctl/internal/flyerr"
	"golang.org/x/crypto/ssh"
)

// ignoredMessages are substrings of errors that are expected outcomes of a
// command (the user cancelled, an account limit, the user's own app failing)
// rather than flyctl bugs. Several of these arrive without the original error
// type in the chain, so they are matched on text.
var ignoredMessages = []string{
	// Ctrl-C or SIGTERM, surfaced through context.Cause by signal.NotifyContext.
	"signal received",
	"deployment aborted by user",
	// Account states the API already explains to the user.
	"reached its machine limit",
	"has overdue invoices",
	// The user's environment or app, with a message telling them what to do.
	"unix socket limit",
	"the app appears to be crashing",
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

	if flyerr.IsCancelledError(err) {
		return true
	}

	// A remote command exiting non-zero, or the server closing the session
	// without an exit status, is the remote side's result, not a client bug.
	var exitErr *ssh.ExitError
	var exitMissing *ssh.ExitMissingError
	if errors.As(err, &exitErr) || errors.As(err, &exitMissing) {
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
