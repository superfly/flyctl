//go:build windows

package setup

import (
	"errors"

	"golang.org/x/sys/windows"
)

// prepareReplace clears FILE_ATTRIBUTE_READONLY so the atomic rename can
// replace the file, then the returned function puts the attribute back on
// whatever is at path afterwards.
func prepareReplace(path string) (func(), error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return func() {}, nil
		}

		return nil, err
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		return func() {}, nil
	}
	if err := windows.SetFileAttributes(ptr, attrs&^windows.FILE_ATTRIBUTE_READONLY); err != nil {
		return nil, err
	}

	return func() {
		_ = windows.SetFileAttributes(ptr, attrs)
	}, nil
}
