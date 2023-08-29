package statuslogger

import (
	"context"
	"sync"

	"github.com/superfly/flyctl/iostreams"
)

func Create(ctx context.Context, numLines int, showStatusChar bool) StatusLogger {

	logNumbers := numLines > 1
	io := iostreams.FromContext(ctx)
	if io.IsInteractive() {

		sl := &interactiveLogger{
			lines:      make([]*interactiveLine, numLines),
			io:         io,
			logNumbers: logNumbers,
			showStatus: showStatusChar,
			active:     true,
			done:       false,
		}

		for i := 0; i < numLines; i++ {
			sl.lines[i] = &interactiveLine{
				logger:  sl,
				lineNum: i,
				status:  StatusNone,
				buf:     "Waiting for job",
			}
		}

		go sl.animateThread()

		return sl
	} else {
		sl := &noninteractiveLogger{
			lines:      make([]*noninteractiveLine, numLines),
			io:         io,
			logNumbers: logNumbers,
			showStatus: showStatusChar,
		}
		for i := 0; i < numLines; i++ {
			sl.lines[i] = &noninteractiveLine{
				logger:  sl,
				lineNum: i,
				status:  StatusNone,
			}
		}
		return sl
	}
}

func asyncIter[T any](ctx context.Context, logger StatusLogger, clearAfter bool, items []T, cb func(context.Context, int, T)) {
	wg := sync.WaitGroup{}
	for i, item := range items {
		wg.Add(1)
		go func(i int, item T) {
			childCtx := NewContext(ctx, logger.Line(i))
			defer wg.Done()
			cb(childCtx, i, item)
		}(i, item)
	}
	wg.Wait()
}

// AsyncIterate runs a callback for each item in a separate goroutine, passing
// a context with a StatusLine for each item.
func AsyncIterate[T any](ctx context.Context, clearAfter bool, items []T, cb func(context.Context, int, T)) {
	logger := Create(ctx, len(items), false)
	defer logger.Destroy(clearAfter)
	asyncIter(ctx, logger, clearAfter, items, cb)
}

// AsyncIterateWithErr runs a callback for each item in a separate goroutine, passing
// a context with a StatusLine for each item. If any callback returns an error,
// the first error is returned and the remaining goroutines are canceled.
// If doneText is non-empty, each line will have its status set to this after its task successfully finishes.
func AsyncIterateWithErr[T any](ctx context.Context, clearAfter bool, doneText string, items []T, cb func(context.Context, int, T) error) error {
	logger := Create(ctx, len(items), true)
	defer logger.Destroy(clearAfter)

	cancelableCtx, done := context.WithCancel(ctx)
	defer done()

	firstErr := make(chan error, 1)
	asyncIter(cancelableCtx, logger, clearAfter, items, func(ctx context.Context, i int, item T) {
		FromContext(ctx).setStatus(StatusRunning)
		err := cb(ctx, i, item)
		if err != nil {
			FromContext(ctx).LogStatus(StatusFailure, err.Error())
			cancelableCtx.Done()
			firstErr <- err
		}
		if doneText != "" {
			FromContext(ctx).LogStatus(StatusSuccess, doneText)
		} else {
			FromContext(ctx).setStatus(StatusSuccess)
		}
	})
	return nil
}

// SingleLine returns a single StatusLine and a function to destroy it.
// Useful for one-off operations.
func SingleLine(ctx context.Context, showStatusChar bool) (context.Context, func(clear bool)) {
	logger := Create(ctx, 1, showStatusChar)
	line := logger.Line(0)
	line.setStatus(StatusRunning)
	return NewContext(ctx, line), logger.Destroy
}
