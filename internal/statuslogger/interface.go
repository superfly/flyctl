package statuslogger

type StatusLogger interface {
	Destroy(clear bool)
	Line(idx int) StatusLine
}

type StatusLine interface {
	Log(s string)
	Logf(format string, args ...interface{})
	LogStatus(s Status, str string)
	LogfStatus(s Status, format string, args ...interface{})
	Failed(e error)
	// Private because it won't redraw on non-interactive loggers.
	// For outside use, use LogStatus or LogfStatus.
	setStatus(s Status)
}
