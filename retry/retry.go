package retry

import (
	"context"
	"time"

	"github.com/jpillora/backoff"
)

// Retry attempts to execute the provided function up to 'attempts' times,
// respecting the context for cancellation and timeout
func Retry(ctx context.Context, fn func() error, attempts uint) (err error) {
	for i := attempts; i > 0; i-- {

		if ctx.Err() != nil {
			return ctx.Err()
		}

		err = fn()
		if err == nil {
			return nil
		}
	}

	return err
}

// Retry attempts to execute the provided function up to 'attempts' times with an
// exponential backoff strategy, respecting the context for cancellation and timeout
func RetryBackoff(ctx context.Context, fn func() error, attempts uint, backoffStrategy *backoff.Backoff) (err error) {
	for i := attempts; i > 0; i-- {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err = fn()
		if err == nil {
			return nil
		}

		select {
		case <-time.After(backoffStrategy.Duration()):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}
