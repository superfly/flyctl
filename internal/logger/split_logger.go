package logger

import "github.com/superfly/flyctl/internal/cmdutil"

type SplitLogger struct {
	terminal logSink
	file     logSink
}

func (l *SplitLogger) WriteLog(level Level, line string) {
	if l.terminal != nil {
		l.terminal.WriteLog(level, line)
	}
	if l.file != nil {
		sanitized := line
		if !l.file.UseAnsi() && l.terminal != nil && l.terminal.UseAnsi() {
			sanitized = cmdutil.StripANSI(line)
		}
		l.file.WriteLog(level, sanitized)
	}
}

func (l *SplitLogger) UseAnsi() bool {
	if l.terminal != nil {
		return l.terminal.UseAnsi()
	}
	if l.file != nil {
		return l.file.UseAnsi()
	}
	return true
}

func (l *SplitLogger) Level() Level {
	if l.terminal != nil {
		return l.terminal.Level()
	}
	if l.file != nil {
		return l.file.Level()
	}
	return NoLogLevel
}
