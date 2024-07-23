//go:build !windows

package synthetics

import (
	"os"
	"os/signal"
	"syscall"
)

func signalChannel(c chan os.Signal) error {
	signal.Notify(c, syscall.SIGUSR1)
	return nil
}
