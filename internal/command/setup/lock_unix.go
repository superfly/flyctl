//go:build unix

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// openLockFile creates or opens the lock without following a symlink.
func openLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|unix.O_NOFOLLOW, 0o600)
}

func lockPathExclusive(lockPath string) (func(), error) {
	f, err := openLockFile(lockPath)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, fmt.Errorf("%s is a symlink; refusing to follow a lock symlink", lockPath)
		}

		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()

		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() {
				_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK) {
			_ = f.Close()

			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()

			return nil, errLockTimeout
		case <-ticker.C:
		}
	}
}
