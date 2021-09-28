package api

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
}