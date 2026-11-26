//go:build !windows

package sprites

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// watchResize calls fn with the terminal's size each time it changes. The
// returned function stops watching.
func watchResize(fd int, fn func(rows, cols uint16)) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ch:
				if width, height, err := term.GetSize(fd); err == nil {
					fn(uint16(height), uint16(width))
				}
			}
		}
	}()

	return func() {
		signal.Stop(ch)
		close(done)
	}
}
