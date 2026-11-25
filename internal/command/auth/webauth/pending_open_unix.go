//go:build !windows

package webauth

import "os"

func openPendingLogin(path string) (*os.File, error) {
	return os.Open(path)
}
