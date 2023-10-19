package statuslogger

type ResumeFn func()

type StatusLogger interface {
	// Destroy destroys the logger.
	// If clear is true, it will remove the status lines from the terminal.
	// Otherwise, it will leave them in place with a clear divider.
	Destroy(clear bool)
	// Line returns a StatusLine for the given line number.
	Line(idx int) StatusLine
	// Pause clears the status lines and prevents redraw until the returned resume function is called.
	// This allows you to write multiple lines to the terminal without overlapping the status area.
	Pause() ResumeFn
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
