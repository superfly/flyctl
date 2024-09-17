package statics

import (
	"context"
	"sync"
)

func spawnWorkers(ctx context.Context, n int, f func(context.Context) error) func() error {
	ctx, cancel := context.WithCancel(ctx)

	workerErr := make(chan error, 1)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(ctx); err != nil {
				cancel()
				select {
				case workerErr <- err:
				default:
				}
			}
		}()
	}

	return func() error {

		defer cancel()
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
