package ctrlc

import (
	"context"

	"github.com/superfly/flyctl/internal/contextutil"
	"github.com/superfly/flyctl/terminal"
)

type customCtx struct {
	context.Context
}

type abortedErr struct{}

func (abortedErr) Error() string { return "aborted by user" }
func (abortedErr) Unwrap() error { return context.Canceled }

func (c *customCtx) Err() error {
	if context.Cause(c.Context) == AbortedByUser {
		return AbortedByUser
	}

	return c.Context.Err()
}

var AbortedByUser = abortedErr{}

// HookContext returns a context that is canceled when the user presses Ctrl+C.
// The context is canceled with AbortedByUser.
// If you're wrapping a context that already has a cancel function, use HookCancelableContext instead.
func HookContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return HookCancelableContext(ctx, func() {})
}

// HookCancelableContext returns a context that is canceled when the user presses Ctrl+C.
// The context is canceled with AbortedByUser.
func HookCancelableContext(ctx context.Context, cancelFn context.CancelFunc) (context.Context, context.CancelFunc) {
	ctx, cancelCause := context.WithCancelCause(ctx)
	newCtx := &customCtx{Context: ctx}

	var handle Handle
	// A signal may arrive before Hook returns its handle.
	ready := make(chan struct{})
	handle = Hook(func() {
		<-ready
		defer handle.Done()
		terminal.Debugf("captured ctrl+c, canceling context")
		cancelCause(AbortedByUser)
		cancelFn()
	})

	close(ready)

	return newCtx, func() {
		handle.Done()
		cancelCause(contextutil.CleanupCause("Ctrl+C hook released"))
		cancelFn()
	}
}
