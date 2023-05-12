package future

import (
	"fmt"
	"sync"
)

type Future[T any] struct {
	mu  sync.RWMutex
	val T
	err error
}

func (fut *Future[T]) Get() (T, error) {
	fut.mu.RLock()
	defer fut.mu.RUnlock()

	return fut.val, fut.err
}

// Spawns `fn` on a new goroutine and returns future which resolves on
// completion.
func Spawn[T any](fn func() (T, error)) *Future[T] {
	// allocate future and lock it immediately, we pass implied ownership of
	// this lock to the spawned goroutine
	fut := new(Future[T])
	fut.mu.Lock()

	// spawn goroutine to call fn and update future when done
	go func() {
		defer func() {
			// if we panicked, set future's error field and rethrow
			if err := recover(); err != nil {
				fut.err = fmt.Errorf("panic: %v", err)
				panic(err)
			}

			fut.mu.Unlock()
		}()

		fut.val, fut.err = fn()
	}()

	return fut
}

func Ready[T any](val T) *Future[T] {
	return &Future[T]{val: val}
}
