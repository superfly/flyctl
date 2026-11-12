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
	"interrupt signal received",
	"terminated signal received",
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
	if err == nil || flyerr.IsCancelledError(err) {
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
