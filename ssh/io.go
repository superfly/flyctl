package ssh

import (
	"context"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	DefaultHeight = 40
	DefaultWidth  = 80
)

var modes = ssh.TerminalModes{
	ssh.ECHO:          0,     // disable echoing
	ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
	ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
}

// FdReader is an io.Reader with an Fd function
type FdReader interface {
	io.Reader
	Fd() uintptr
}

type SessionIO struct {
	Stdin  io.Reader
	Stdout io.WriteCloser
	Stderr io.WriteCloser

	AllocPTY bool
	TermEnv  string
}

func getFd(reader io.Reader) (fd int, ok bool) {
	fdthing, ok := reader.(FdReader)
	if !ok {
		return 0, false
	}

	fd = int(fdthing.Fd())
	return fd, term.IsTerminal(fd)
}

func (s *SessionIO) attach(ctx context.Context, sess *ssh.Session, cmd string) error {

	if s.AllocPTY {
		width, height := DefaultWidth, DefaultHeight

		if fd, ok := getFd(s.Stdin); ok {
			state, err := term.MakeRaw(fd)
			if err != nil {
				return err
			}
			defer term.Restore(fd, state)
		}

		if w, h, err := s.getAndWatchSize(ctx, sess); err == nil {
			width, height = w, h
		}

		if err := sess.RequestPty(s.TermEnv, height, width, modes); err != nil {
			return err
		}
	}

	var closeStdin sync.Once
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	defer closeStdin.Do(func() {
		stdin.Close()
	})

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
	}

	go func() {
		defer closeStdin.Do(func() {
			stdin.Close()
		})
		if s.Stdin != nil {
			io.Copy(stdin, s.Stdin)
		}
	}()
	if s.Stdout != nil {
		go io.Copy(s.Stdout, stdout)
	}

	if s.Stderr != nil {
		go io.Copy(s.Stderr, stderr)
	}

	cmdC := make(chan error, 1)
	go func() {
		defer close(cmdC)
		if cmd == "" {
			err = sess.Shell()
		} else {
			err = sess.Run(cmd)
		}
		if err != nil && err != io.EOF {
			cmdC <- err
		}
	}()

	select {
	case err := <-cmdC:
		return err
	case <-ctx.Done():
		return errors.New("session forcibly closed; the remote process may still be running")
	}
}
