//go:build windows

package synthetics

import (
	"os"
)

func signalChannel(c chan os.Signal) error {
	return nil
}
