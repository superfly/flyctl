//go:build windows

package setup

import (
	"errors"
	"os"
)

// prepareReplace clears FILE_ATTRIBUTE_READONLY so the atomic rename can
// replace the file, then restores it. Root.Chmod uses a handle beneath the
// pinned directory rather than resolving the original absolute pathname.
func prepareReplace(dir *os.Root, name string) (func(), error) {
	info, err := dir.Stat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return func() {}, nil
		}

		return nil, err
	}
	if info.Mode().Perm()&0o200 != 0 {
		return func() {}, nil
	}
	if err := dir.Chmod(name, info.Mode().Perm()|0o200); err != nil {
		return nil, err
	}

	return func() {
		_ = dir.Chmod(name, info.Mode().Perm())
	}, nil
}
