//go:build !windows
// +build !windows

package wg

import (
	"os"
	"os/signal"
	"syscall"
)

func signalChannel(c chan os.Signal) error {
	signal.Notify(c, syscall.SIGUSR1)
	return nil
}
