package ctrlc

import (
	"context"
	"sync/atomic"

	"github.com/superfly/flyctl/terminal"
)

type customCtx struct {
	context.Context
	signalTripped atomic.Bool
}

type abortedErr struct{}

func (abortedErr) Error() string { return "aborted by user" }
func (abortedErr) Unwrap() error { return context.Canceled }

func (c *customCtx) Err() error {
	if c.signalTripped.Load() {
		return AbortedByUser
	}
	return c.Context.Err()
}

var AbortedByUser = abortedErr{}

// HookContext returns a context that is canceled when the user presses Ctrl+C.
// The context is canceled with AbortedByUser.
// If you're wrapping a context that already has a cancel function, use HookCancelableContext instead.
func HookContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancelFn := context.WithCancel(ctx)
	return HookCancelableContext(ctx, cancelFn)
}

// HookCancelableContext returns a context that is canceled when the user presses Ctrl+C.
// The context is canceled with AbortedByUser.
func HookCancelableContext(ctx context.Context, cancelFn context.CancelFunc) (context.Context, context.CancelFunc) {
	var handle Handle

	newCtx := &customCtx{Context: ctx}

	handle = Hook(func() {
		terminal.Debugf("captured ctrl+c, canceling context")
		newCtx.signalTripped.Store(true)
		cancelFn()
		handle.Done()
	})
	return newCtx, func() {
		handle.Done()
		cancelFn()
	}
}
