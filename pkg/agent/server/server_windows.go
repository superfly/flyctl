//go:build windows
// +build windows

package server

import (
	"errors"
	"io/fs"
	"os"
)

func removeSocket(path string) (err error) {
	switch _, err = os.Lstat(path); {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
		break
	default:
		if err = os.Remove(path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}

	return
}
