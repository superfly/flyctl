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

func Debug(v ...interface{}) {
	DefaultLogger.Debug(v...)
}

func Debugf(format string, v ...interface{}) {
	DefaultLogger.Debugf(format, v...)
}

func Info(v ...interface{}) {
	DefaultLogger.Info(v...)
}

func Infof(format string, v ...interface{}) {
	DefaultLogger.Infof(format, v...)
}

func Warn(v ...interface{}) {
	DefaultLogger.Warn(v...)
}

func Warnf(format string, v ...interface{}) {
	DefaultLogger.Warnf(format, v...)
}

func Error(v ...interface{}) {
	DefaultLogger.Error(v...)
}

func Errorf(format string, v ...interface{}) {
	DefaultLogger.Errorf(format, v...)
}
