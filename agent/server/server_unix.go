//go:build !windows
// +build !windows

package server

import (
	"errors"
	"io/fs"
	"os"
)

func removeSocket(path string) (err error) {
	var stat os.FileInfo
	switch stat, err = os.Lstat(path); {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
		break
	case stat.Mode()&os.ModeSocket == 0:
		err = errors.New("not a socket")
	default:
		if err = os.Remove(path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}

	return
}
