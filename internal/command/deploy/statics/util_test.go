package statics

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpawnWorkersPreservesFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failure := errors.New("statics upload failed")
	var started atomic.Int32
	ready := make(chan struct{})
	observed := make(chan error, 1)
	wait := spawnWorkers(ctx, 2, func(ctx context.Context) error {
		if started.Add(1) == 1 {
			select {
			case <-ready:
			case <-ctx.Done():
				return ctx.Err()
			}

			return failure
		}
		close(ready)
		<-ctx.Done()
		observed <- context.Cause(ctx)

		return ctx.Err()
	})
	if err := wait(); err != failure {
		t.Fatalf("worker failure replaced: %v", err)
	}
	if cause := <-observed; cause != failure {
		t.Fatalf("sibling cause = %v", cause)
	}
}
