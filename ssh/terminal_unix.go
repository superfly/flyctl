//go:build !windows
// +build !windows

package ssh

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func watchWindowSize(ctx context.Context, fd int, sess *ssh.Session) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGWINCH)

	for {
		select {
		case <-sigc:
		case <-ctx.Done():
			return nil
		}

		width, height, err := term.GetSize(fd)
		if err != nil {
			return err
		}

		if err := sess.WindowChange(height, width); err != nil {
			return err
		}
	}
}
