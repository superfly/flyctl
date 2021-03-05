package ssh

import (
	"context"
	"io"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
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

func (t *Terminal) attach(ctx context.Context, sess *ssh.Session) error {
	width, height := DefaultWidth, DefaultHeight
	if fd := int(t.Stdin.Fd()); terminal.IsTerminal(fd) {
		state, err := terminal.MakeRaw(fd)
		if err != nil {
			return err
		}
		defer terminal.Restore(fd, state)

		width, height, err = terminal.GetSize(fd)
		if err != nil {
			return err
		}

		go watchWindowSize(ctx, fd, sess)
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

	if err := sess.Shell(); err != nil && err != io.EOF {
		return err
	}
	return nil
}
