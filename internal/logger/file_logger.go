package logger

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync"
)

type logFile struct {
	file      *os.File
	writer    *bufio.Writer
	lock      sync.Mutex
	destroyed bool
}

var (
	logfileAlreadyClosedError      = errors.New("logfile already closed")
	logfileAlreadyInitializedError = errors.New("logfile already initialized")
)

func (l *logFile) WriteLog(_ Level, line string) {
	fmt.Fprint(l, line)
}

func (l *logFile) UseAnsi() bool {
	return false
}

func (l *logFile) Write(p []byte) (n int, err error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.destroyed {
		return 0, logfileAlreadyClosedError
	}
	return l.writer.Write(p)
}

func (l *logFile) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.destroyed {
		return logfileAlreadyClosedError
	}
	l.destroyed = true
	if err := l.writer.Flush(); err != nil {
		return err
	}
	return l.file.Close()
}

func (l *logFile) Level() Level {
	return NoLogLevel
}
