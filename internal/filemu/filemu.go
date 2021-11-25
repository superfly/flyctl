// Package filemu implements file-based mutexes.
package filemu

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/flock"
)

// Unlocker is the interface that wraps the basic Unlock method.
type Unlocker interface {
	Unlock() error
}

const (
	timeout    = time.Second
	retryDelay = timeout / 10
)

// Lock attempts to acquire an exclusive lock on the named file.
func Lock(ctx context.Context, path string) (Unlocker, error) {
	return try(ctx, path, (*flock.Flock).TryLockContext)
}

// RLock attempts to acquire a shared lock on the named file.
func RLock(ctx context.Context, path string) (Unlocker, error) {
	return try(ctx, path, (*flock.Flock).TryRLockContext)
}

var errFailed = errors.New("failed acquiring lock")

type unlockFunc func() error

func (fn unlockFunc) Unlock() error {
	return fn()
}

type lockFunc func(*flock.Flock, context.Context, time.Duration) (bool, error)

func try(parent context.Context, path string, fn lockFunc) (Unlocker, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	mu := flock.New(path)

	switch locked, err := fn(mu, ctx, retryDelay); {
	case err != nil:
		return nil, err
	case !locked:
		return nil, errFailed
	default:
		return unlockFunc(mu.Unlock), nil
	}
}
