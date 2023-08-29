package statuslogger

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/iostreams"
)

type noninteractiveLogger struct {
	io         *iostreams.IOStreams
	lines      []*noninteractiveLine
	logNumbers bool
	showStatus bool
}

func (sl *noninteractiveLogger) Line(i int) StatusLine {
	return sl.lines[i]
}

// Destroy is a no-op for non-interactive loggers.
func (sl *noninteractiveLogger) Destroy(_ bool) {}

type noninteractiveLine struct {
	logger  *noninteractiveLogger
	lineNum int
	status  Status
}

func (sl *noninteractiveLine) Log(s string) {
	buf := ""
	if sl.logger.showStatus {
		buf += sl.status.charFor(-1) + " "
	}
	if sl.logger.logNumbers {
		buf += formatIndex(sl.lineNum, len(sl.logger.lines)) + " "
	}
	buf += s
	fmt.Fprintln(sl.logger.io.Out, buf)
}

func (sl *noninteractiveLine) Logf(format string, args ...interface{}) {
	sl.Log(fmt.Sprintf(format, args...))
}

func (sl *noninteractiveLine) LogStatus(s Status, str string) {
	sl.status = s
	sl.Log(str)
}

func (sl *noninteractiveLine) LogfStatus(s Status, format string, args ...interface{}) {
	sl.LogStatus(s, fmt.Sprintf(format, args...))
}

func (sl *noninteractiveLine) Failed(e error) {
	firstLine, _, _ := strings.Cut(e.Error(), "\n")
	sl.LogfStatus(StatusFailure, "Failed: %s", firstLine)
}

func (sl *noninteractiveLine) setStatus(s Status) {
	sl.status = s
}

func (sl *noninteractiveLogger) Pause() ResumeFn { return func() {} }
