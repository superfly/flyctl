package terminal

import (
	"os"
	"strings"

	"github.com/superfly/flyctl/internal/logger"
)

var DefaultLogger *logger.Logger

func init() {

	var level logger.Level

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = logger.Debug
	case "info":
		level = logger.Info
	case "warn":
		level = logger.Warn
	case "error":
		level = logger.Error
	default:
		level = logger.Info
	}

	DefaultLogger = logger.New(os.Stdout, level, true).AndLogToFile()
}

func GetLogLevel() logger.Level {
	return DefaultLogger.Level()
}

func Debug(v ...any) {
	DefaultLogger.Debug(v...)
}

func Debugf(format string, v ...any) {
	DefaultLogger.Debugf(format, v...)
}

func Info(v ...any) {
	DefaultLogger.Info(v...)
}

func Infof(format string, v ...any) {
	DefaultLogger.Infof(format, v...)
}

func Warn(v ...any) {
	DefaultLogger.Warn(v...)
}

func Warnf(format string, v ...any) {
	DefaultLogger.Warnf(format, v...)
}

func Error(v ...any) {
	DefaultLogger.Error(v...)
}

func Errorf(format string, v ...any) {
	DefaultLogger.Errorf(format, v...)
}
