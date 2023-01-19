package ssh

import (
	"context"
	"fmt"
	"io"
	"runtime"

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

// FdWriteCloser is an io.WriteCloser with an Fd function
type FdWriteCloser interface {
	io.WriteCloser
	Fd() uintptr
}

type Terminal struct {
	Stdin  io.Reader
	Stdout io.WriteCloser
	Stderr io.WriteCloser

	Mode string
}

func getFd(reader io.Reader) (fd int, ok bool) {
	fdthing, ok := reader.(FdReader)
	if !ok {
		return 0, false
	}

	fd = int(fdthing.Fd())
	return fd, term.IsTerminal(fd)
}

func (t *Terminal) attach(ctx context.Context, sess *ssh.Session, cmd string, workdir string) error {
	width, height := DefaultWidth, DefaultHeight
	if fd, ok := getFd(t.Stdin); ok {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		defer term.Restore(fd, state)

		// BUG(tqbf): this is a temporary hack to work around a windows
		// terminal handling problem that is probably trivial to fix, but
		// winch isn't handled yet there anyways
		if runtime.GOOS != "windows" {
			width, height, err = term.GetSize(fd)
			if err != nil {
				return err
			}

			go watchWindowSize(ctx, fd, sess)
		}
	}

	if err := sess.RequestPty(t.Mode, height, width, modes); err != nil {
		return err
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()

	if workdir != "" {
		cd := fmt.Sprintf("cd %s\n", workdir)
		_, err := stdin.Write([]byte(cd))
		if err != nil {
			return err
		}
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
	}

	if workdir != "" {
		cd := fmt.Sprintf("cd %s\n", workdir)
		_, err := stdin.Write([]byte(cd))
		if err != nil {
			return err
		}
	}

	go io.Copy(stdin, t.Stdin)
	go io.Copy(t.Stdout, stdout)
	go io.Copy(t.Stderr, stderr)

	if cmd == "" {
		err = sess.Shell()
	} else {
		err = sess.Run(cmd)
	}
	if err != nil && err != io.EOF {
		return err
	}

	return nil
}
