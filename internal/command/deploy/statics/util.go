package statics

import (
	"context"
	"sync"
)

func spawnWorkers(ctx context.Context, n int, f func(context.Context) error) func() error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerErr := make(chan error, 1)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(ctx); err != nil {
				cancel()
				workerErr <- err
			}
		}()
	}

	return func() error {

		wg.Wait()

		// Check if any of the workers failed.
		select {
		case err := <-workerErr:
			return err
		default:
			return nil
		}
	}
}
