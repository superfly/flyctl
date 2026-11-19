package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
)

// errLockTimeout is returned when the config lock is still held after
// fileLockTimeout. Callers map it to the "timed out waiting" message.
var errLockTimeout = errors.New("timed out waiting for config lock")

// configLockPath is a lock file in a 0700 directory under the flyctl state
// dir, named by a hash of the unresolved config path. The lock is not placed
// beside the config, so a pre-planted symlink there cannot redirect it and
// dotfile repos do not collect *.lock files.
func configLockPath(logical string) (string, error) {
	stateDir, err := helpers.GetConfigDirectory()
	if err != nil {
		return "", err
	}
	lockDir := filepath.Join(stateDir, "setup-locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(lockDir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(logical)))

	return filepath.Join(lockDir, hex.EncodeToString(sum[:])+".lock"), nil
}

// acquireConfigLock takes an exclusive lock for logical. A symlink at the
// lock path is refused before it is opened. The lock file stays in the state
// directory after the run.
func acquireConfigLock(logical string) (func(), error) {
	lockPath, err := configLockPath(logical)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(lockPath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		return nil, fmt.Errorf("%s is a symlink; refusing to follow a lock symlink", lockPath)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return nil, err
	}

	return lockPathExclusive(lockPath)
}
