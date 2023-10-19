//go:build !windows
// +build !windows

package ssh

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/superfly/flyctl/terminal"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func (s *SessionIO) getAndWatchSize(ctx context.Context, sess *ssh.Session) (int, int, error) {
	fd, ok := getFd(s.Stdin)
	if !ok {
		return 0, 0, errors.New("could not get console handle")
	}

	width, height, err := term.GetSize(fd)
	if err != nil {
		return 0, 0, err
	}

	go func() {
		if err := watchWindowSize(ctx, fd, sess); err != nil {
			terminal.Debugf("Error watching window size: %s\n", err)
		}
	}()

	return width, height, nil
}

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
