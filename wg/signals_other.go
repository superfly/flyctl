//go:build windows || js

package wg

import (
	"os"
)

func signalChannel(c chan os.Signal) error {
	return nil
}
