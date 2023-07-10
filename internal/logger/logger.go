package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/chroma/quick"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/iostreams"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error

	NoLogLevel = -1
)

type logSink interface {
	WriteLog(level Level, line string)
	UseAnsi() bool
	Level() Level
}

type Logger struct {
	inner logSink
}

func FromEnv(out io.Writer) *Logger {
	return &Logger{inner: &WriterLogger{
		out:    out,
		level:  levelFromEnv(),
		isTerm: iostreams.IsTerminalWriter(out),
	}}
}

func New(out io.Writer, level Level, isTerm bool) *Logger {
	return &Logger{inner: &WriterLogger{
		out:    out,
		level:  level,
		isTerm: isTerm,
	}}
}

func levelFromEnv() Level {
	lit, ok := os.LookupEnv("LOG_LEVEL")
	if !ok && buildinfo.IsDev() {
		lit = "warn"
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

func (l *Logger) write(level Level, prefix aurora.Value, v ...any) {
	if l == nil {
		return
	}
	line := fmt.Sprintf("%s %s\n", prefix.String(), fmt.Sprint(v...))
	if !l.inner.UseAnsi() {
		line = cmdutil.StripANSI(line)
	}
	l.inner.WriteLog(level, line)
}

func (l *Logger) Debug(v ...interface{}) {
	if l == nil {
		return
	}
	if str, ok := v[0].(string); ok {
		byteString := []byte(str)
		if json.Valid(byteString) {
			var prettyJSON bytes.Buffer
			err := json.Indent(&prettyJSON, byteString, "", "  ")
			if err == nil {
				jsonStr := prettyJSON.String() + "\n"
				if l.inner.UseAnsi() {
					outBuf := &bytes.Buffer{}
					quick.Highlight(outBuf, prettyJSON.String()+"\n", "json", "terminal", "monokai")
					jsonStr = outBuf.String()
				}
				l.write(Debug, aurora.Faint("DEBUG"), jsonStr)
				return
			}
		}
	}

	l.write(Debug, aurora.Faint("DEBUG"), v...)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.Debug(fmt.Sprintf(format, v...))
}

func (l *Logger) Info(v ...interface{}) {
	l.write(Info, aurora.Faint("INFO"), v...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.Info(fmt.Sprintf(format, v...))
}

func (l *Logger) Warn(v ...interface{}) {
	l.write(Warn, aurora.Yellow("WARN"), v...)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.Warn(fmt.Sprintf(format, v...))
}

func (l *Logger) Error(v ...interface{}) {
	l.write(Error, aurora.Red("ERROR"), v...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.Error(fmt.Sprintf(format, v...))
}

func (l *Logger) AndLogToFile() *Logger {
	return &Logger{inner: &SplitLogger{
		terminal: l.inner,
		file:     &globalLogFile,
	}}
}

// Level returns the current log level, or NoLogLevel if not applicable
func (l *Logger) Level() Level {
	return l.inner.Level()
}
