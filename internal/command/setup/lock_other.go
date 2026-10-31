//go:build !unix

package setup

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gofrs/flock"
)

// openLockFile opens the lock after acquireConfigLock has refused a symlink.
// These platforms have no O_NOFOLLOW; the lock still lives in the 0700 state
// directory rather than beside the config.
func openLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

func lockPathExclusive(lockPath string) (func(), error) {
	mu := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	locked, err := mu.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil || !locked {
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}

		return nil, errLockTimeout
	}

	return func() { _ = mu.Unlock() }, nil
}
