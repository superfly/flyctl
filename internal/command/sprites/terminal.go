package sprites

import (
	"io"
	"os"

	"golang.org/x/term"
)

// localTerminal is the terminal a TTY session is wired to.
type localTerminal interface {
	// Input is what the user types.
	Input() io.Reader

	// MakeRaw passes keystrokes through unprocessed, so that the remote
	// terminal handles them. The returned function undoes it.
	MakeRaw() (restore func(), err error)

	// Size reports the terminal's size, when it can be read.
	Size() (rows, cols uint16, ok bool)

	// WatchResize calls fn with the new size each time it changes. The
	// returned function stops watching.
	WatchResize(fn func(rows, cols uint16)) (stop func())
}

// openTerminal returns the terminal on stdin, or nil when stdin is not one.
// It is a variable so tests can supply a terminal.
var openTerminal = func() localTerminal {
	stdin := int(os.Stdin.Fd())
	if !term.IsTerminal(stdin) {
		return nil
	}

	// The size is asked of stdout where possible: on Windows only an output
	// handle can answer.
	sizeFd := stdin
	if stdout := int(os.Stdout.Fd()); term.IsTerminal(stdout) {
		sizeFd = stdout
	}

	return &stdinTerminal{fd: stdin, sizeFd: sizeFd}
}

type stdinTerminal struct {
	fd     int
	sizeFd int
}

func (t *stdinTerminal) Input() io.Reader { return os.Stdin }

func (t *stdinTerminal) MakeRaw() (func(), error) {
	state, err := term.MakeRaw(t.fd)
	if err != nil {
		return nil, err
	}

	return func() { _ = term.Restore(t.fd, state) }, nil
}

func (t *stdinTerminal) Size() (rows, cols uint16, ok bool) {
	width, height, err := term.GetSize(t.sizeFd)
	if err != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}

	return uint16(height), uint16(width), true
}

func (t *stdinTerminal) WatchResize(fn func(rows, cols uint16)) func() {
	return watchResize(t.sizeFd, fn)
}
