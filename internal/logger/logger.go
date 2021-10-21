package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/internal/buildinfo"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Logger struct {
	out   io.Writer
	level Level
}

func FromEnv(out io.Writer) *Logger {
	return &Logger{
		out:   out,
		level: levelFromEnv(),
	}
}

func levelFromEnv() Level {
	lit, ok := os.LookupEnv("LOG_LEVEL")
	if !ok && buildinfo.IsDev() {
		lit = "debug"
	} else {
		lit = strings.ToLower(lit)
	}

	switch lit {
	default:
		return Info
	case "debug":
		return Debug
	case "warn":
		return Warn
	case "error":
		return Error
	}
}

func (l *Logger) debug(v ...interface{}) {
	fmt.Fprintln(
		l.out,
		aurora.Faint("DEBUG"),
		fmt.Sprint(v...),
	)
}

func (l *Logger) Debug(v ...interface{}) {
	if l.level <= Debug {
		l.debug(v...)
	}
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.level <= Debug {
		l.debug(fmt.Sprintf(format, v...))
	}
}

func (l *Logger) info(v ...interface{}) {
	fmt.Fprintln(
		l.out,
		aurora.Faint("INFO"),
		fmt.Sprint(v...),
	)
}

func (l *Logger) Info(v ...interface{}) {
	if l.level <= Info {
		l.info(v...)
	}
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if l.level <= Info {
		l.info(fmt.Sprintf(format, v...))
	}
}

func (l *Logger) warn(v ...interface{}) {
	fmt.Fprintln(
		l.out,
		aurora.Yellow("WARN"),
		fmt.Sprint(v...),
	)
}

func (l *Logger) Warn(v ...interface{}) {
	if l.level <= Warn {
		l.warn(v...)
	}
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.level <= Warn {
		l.warn(fmt.Sprintf(format, v...))
	}
}

func (l *Logger) error(v ...interface{}) {
	fmt.Fprintln(
		l.out,
		aurora.Red("WARN"),
		fmt.Sprint(v...),
	)
}

func (l *Logger) Error(v ...interface{}) {
	if l.level <= Error {
		l.error(v...)
	}
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.level <= Error {
		l.error(fmt.Sprintf(format, v...))
	}
}
