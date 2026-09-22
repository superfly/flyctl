// Package contextutil provides cancellation with a reason for normal cleanup.
package contextutil

import "context"

// CleanupCause identifies expected shutdown or completion, rather than failure.
// It remains compatible with errors.Is(err, context.Canceled).
type CleanupCause string

func (c CleanupCause) Error() string { return string(c) }
func (CleanupCause) Unwrap() error   { return context.Canceled }

// WithCancel is like context.WithCancel, but records why the owner stops the
// operation. Parent cancellation (including an earlier failure) takes precedence.
// Use context.WithCancelCause directly when cancellation can carry a failure.
func WithCancel(parent context.Context, reason string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	return ctx, func() { cancel(CleanupCause(reason)) }
}
