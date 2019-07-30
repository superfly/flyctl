package terminal

import (
	"fmt"

	"github.com/logrusorgru/aurora"
)

type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelCritical
	LevelFatal
)

var level = LevelWarn

func SetLogLevel(lvl LogLevel) {
	level = lvl
}

func Debug(v ...interface{}) {
	if level > LevelDebug {
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
	if level > LevelDebug {
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
	if level > LevelInfo {
		return
	}
	fmt.Print("INFO ")
	fmt.Println(v...)
}

func Infof(format string, v ...interface{}) {
	if level > LevelInfo {
		return
	}
	fmt.Print("INFO ")
	fmt.Printf(format, v...)
}

func Warn(v ...interface{}) {
	if level > LevelWarn {
		return
	}
	fmt.Print(aurora.Yellow("WARN "))
	fmt.Println(v...)
}

func Warnf(format string, v ...interface{}) {
	if level > LevelWarn {
		return
	}
	fmt.Print(aurora.Yellow("WARN "))
	fmt.Printf(format, v...)
}

func Error(v ...interface{}) {
	if level > LevelError {
		return
	}
	fmt.Print(aurora.Red("ERROR "))
	fmt.Println(v...)
}

func Errorf(format string, v ...interface{}) {
	if level > LevelError {
		return
	}
	fmt.Print(aurora.Red("ERROR "))
	fmt.Printf(format, v...)
}
