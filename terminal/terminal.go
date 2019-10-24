package terminal

import (
	"bytes"
	"fmt"
	"io"

	"github.com/morikuni/aec"
)

func NewTerminal(w io.Writer) *Terminal {
	return &Terminal{w: w}
}

type Terminal struct {
	w         io.Writer
	buf       bytes.Buffer
	pos       uint
	overwrite uint
}

var sep = []byte("\n")
var overwriteSep = []byte("\n")

func (l *Terminal) Write(p []byte) (n int, err error) {
	l.pos += uint(bytes.Count(p, sep))
	return l.w.Write(p)
}

func (l *Terminal) Overwrite() {
	fmt.Fprint(l.w, aec.Up(l.pos))
	for line := l.pos; line > 0; line-- {
		fmt.Fprint(l.w, aec.Column(0))
		fmt.Fprint(l.w, aec.EraseLine(aec.EraseModes.All))
		fmt.Fprintln(l.w)
	}
	fmt.Fprint(l.w, aec.Up(l.pos))
	l.pos = 0
}

func (l *Terminal) ResetPosition() {
	l.pos = 0
}

func (l *Terminal) HideCursor() {
	fmt.Fprint(l.w, "\033[?25l")
}

func (l *Terminal) ShowCursor() {
	fmt.Fprint(l.w, "\033[?25h")
}

func (l *Terminal) Up(n uint) {
	fmt.Fprint(l.w, aec.Up(n))
}

func (l *Terminal) Column(n uint) {
	fmt.Fprint(l.w, aec.Column(n))
}
