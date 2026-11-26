//go:build windows

package sprites

import (
	"time"

	"golang.org/x/term"
)

// watchResize calls fn with the terminal's size each time it changes. Windows
// has no resize signal, so the size is polled. The returned function stops
// watching.
func watchResize(fd int, fn func(rows, cols uint16)) func() {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		lastWidth, lastHeight, _ := term.GetSize(fd)

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				width, height, err := term.GetSize(fd)
				if err != nil || (width == lastWidth && height == lastHeight) {
					continue
				}

				lastWidth, lastHeight = width, height
				fn(uint16(height), uint16(width))
			}
		}
	}()

	return func() { close(done) }
}
