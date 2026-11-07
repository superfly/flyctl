package helpers

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// IsFileBusy reports whether err, from an operation on the file at path,
// means another process has the file open or is replacing it, so the same
// operation may succeed in a moment.
//
// Windows reports a file being replaced or deleted elsewhere as "access
// denied", the same error it gives for a file that can never be written.
// Those are told apart by what is at path: a directory or a read-only file
// stays that way, so retrying would only delay the failure.
func IsFileBusy(path string, err error) bool {
	switch {
	case errors.Is(err, windows.ERROR_SHARING_VIOLATION), errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return true
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		info, statErr := os.Stat(path)

		// A file pending deletion can't be stat'ed either: that one is busy.
		return statErr != nil || (!info.IsDir() && info.Mode().Perm()&0o200 != 0)
	default:
		return false
	}
}
