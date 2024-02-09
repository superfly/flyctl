//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/pprof"

	"golang.org/x/sys/unix"
)

// handleDebugSignal handles SIGUSR2 and dumps debug information.
func handleDebugSignal(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGUSR2)

	for {
		select {
		case <-sigCh:
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		case <-ctx.Done():
			return
		}
	}
}
