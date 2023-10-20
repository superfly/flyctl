package logger

import (
	"fmt"
	"io"
	"time"
)

var startTime = time.Now()

type WriterLogger struct {
	out    io.Writer
	level  Level
	isTerm bool
}

func (l *WriterLogger) WriteLog(level Level, line string) {
	if level < l.level {
		return
	}
	line = fmt.Sprintf("%s %s", time.Since(startTime), line)
	fmt.Fprint(l.out, line)
}

func (l *WriterLogger) UseAnsi() bool {
	return l.isTerm
}

func (l *WriterLogger) Level() Level {
	return l.level
}
