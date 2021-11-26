// Package filemu implements file-based mutexes.
package filemu

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/flock"
)

// UnlockFunc is the set of unlock functions.
type UnlockFunc func() error

const (
	timeout    = time.Second
	retryDelay = timeout / 10
)

// Lock attempts to acquire an exclusive lock on the named file.
func Lock(ctx context.Context, path string) (UnlockFunc, error) {
	return try(ctx, path, (*flock.Flock).TryLockContext)
}

// RLock attempts to acquire a shared lock on the named file.
func RLock(ctx context.Context, path string) (UnlockFunc, error) {
	return try(ctx, path, (*flock.Flock).TryRLockContext)
}

var errFailed = errors.New("failed acquiring lock")

type lockFunc func(*flock.Flock, context.Context, time.Duration) (bool, error)

func try(parent context.Context, path string, fn lockFunc) (UnlockFunc, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	mu := flock.New(path)

	switch locked, err := fn(mu, ctx, retryDelay); {
	case err != nil:
		return nil, err
	case !locked:
		return nil, errFailed
	default:
		return mu.Unlock, nil
	}
}
