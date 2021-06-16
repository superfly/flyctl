package ssh

import (
	"context"
	"io"
	"runtime"
	"os"

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

type Terminal struct {
	Stdin, Stdout, Stderr *os.File

	Mode string
}

func (t *Terminal) attach(ctx context.Context, sess *ssh.Session, cmd string) error {
	width, height := DefaultWidth, DefaultHeight
	if fd := int(t.Stdin.Fd()); term.IsTerminal(fd) {
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

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
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
