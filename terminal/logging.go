package terminal

import (
	"fmt"
	"os"
	"strings"

	"github.com/logrusorgru/aurora"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var DefaultLogger = &Logger{level: LevelInfo}

type Logger struct {
	level LogLevel
}

func init() {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		DefaultLogger.SetLogLevel(LevelDebug)
	case "info":
		DefaultLogger.SetLogLevel(LevelInfo)
	case "warn":
		DefaultLogger.SetLogLevel(LevelWarn)
	case "error":
		DefaultLogger.SetLogLevel(LevelError)
	default:
		DefaultLogger.SetLogLevel(LevelInfo)
	}
}

func (l *Logger) SetLogLevel(lvl LogLevel) {
	l.level = lvl
}

func Debug(v ...interface{}) {
	DefaultLogger.Debug(v...)
}

func (l *Logger) Debug(v ...interface{}) {
	if l.level > LevelDebug {
		return
	}

	fmt.Println(
		aurora.Sprintf(
			aurora.Faint("DEBUG %s"),
			fmt.Sprint(v...),
		),
	)
}

func Debugf(format string, v ...interface{}) {
	DefaultLogger.Debugf(format, v...)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.level > LevelDebug {
		return
	}

	fmt.Printf(
		aurora.Sprintf(
			aurora.Faint(fmt.Sprintf("DEBUG %s", format)),
			v...,
		),
	)
}

func Info(v ...interface{}) {
	DefaultLogger.Info(v...)
}

func (l *Logger) Info(v ...interface{}) {
	if l.level > LevelInfo {
		return
	}
	fmt.Print("INFO ")
	fmt.Println(v...)
}

func Infof(format string, v ...interface{}) {
	DefaultLogger.Infof(format, v...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if l.level > LevelInfo {
		return
	}
	fmt.Print("INFO ")
	fmt.Printf(format, v...)
}

func Warn(v ...interface{}) {
	DefaultLogger.Warn(v...)
}

func (l *Logger) Warn(v ...interface{}) {
	if l.level > LevelWarn {
		return
	}
	fmt.Print(aurora.Yellow("WARN "))
	fmt.Println(v...)
}

func Warnf(format string, v ...interface{}) {
	DefaultLogger.Warnf(format, v...)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.level > LevelWarn {
		return
	}
	fmt.Print(aurora.Yellow("WARN "))
	fmt.Printf(format, v...)
}

func Error(v ...interface{}) {
	DefaultLogger.Error(v...)
}

func (l *Logger) Error(v ...interface{}) {
	if l.level > LevelError {
		return
	}
	fmt.Print(aurora.Red("ERROR "))
	fmt.Println(v...)
}

func Errorf(format string, v ...interface{}) {
	DefaultLogger.Errorf(format, v...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.level > LevelError {
		return
	}
	fmt.Print(aurora.Red("ERROR "))
	fmt.Printf(format, v...)
}
