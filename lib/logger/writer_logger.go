package logger

import (
	"fmt"
	"io"
)

type WriterLogger struct {
	out    io.Writer
	level  Level
	isTerm bool
}

func (l *WriterLogger) WriteLog(level Level, line string) {
	if level < l.level {
		return
	}
	fmt.Fprint(l.out, line)
}

func (l *WriterLogger) UseAnsi() bool {
	return l.isTerm
}

func (l *WriterLogger) Level() Level {
	return l.level
}
