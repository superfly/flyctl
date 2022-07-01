//go:build windows
// +build windows

package wg

import (
	"os"
)

func signalChannel(c chan os.Signal) error {
	return nil
}
