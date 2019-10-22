package terminal

import (
	"bytes"
	"io"
)

type LineCounter struct {
	W io.Writer
	n uint
}

func (l *LineCounter) Write(p []byte) (int, error) {
	l.n += uint(bytes.Count(p, []byte("\n")))
	return l.W.Write(p)
}

func (l *LineCounter) LinesWritten() uint {
	return l.n
}

func (l *LineCounter) Reset() {
	l.n = 0
}
