package statuslogger

import (
	"fmt"
	"strings"
	"sync"

	"github.com/superfly/flyctl/iostreams"
)

type noninteractiveLogger struct {
	// mu protects io.
	mu sync.Mutex
	io *iostreams.IOStreams

	logNumbers bool
	showStatus bool
	lines      []*noninteractiveLine
}

func (nl *noninteractiveLogger) Line(i int) StatusLine {
	return nl.lines[i]
}

// Destroy is a no-op for non-interactive loggers.
func (nl *noninteractiveLogger) Destroy(_ bool) {}

type noninteractiveLine struct {
	logger  *noninteractiveLogger
	lineNum int
	status  Status
}

func (line *noninteractiveLine) Log(s string) {
	buf := ""
	if line.logger.showStatus {
		buf += line.status.charFor(-1) + " "
	}
	if line.logger.logNumbers {
		buf += formatIndex(line.lineNum, len(line.logger.lines)) + " "
	}
	buf += s

	line.println(buf)
}

func (line *noninteractiveLine) println(s string) {
	line.logger.mu.Lock()
	defer line.logger.mu.Unlock()
	fmt.Fprintln(line.logger.io.Out, s)
}

func (line *noninteractiveLine) Logf(format string, args ...interface{}) {
	line.Log(fmt.Sprintf(format, args...))
}

func (line *noninteractiveLine) LogStatus(s Status, str string) {
	line.status = s
	line.Log(str)
}

func (line *noninteractiveLine) LogfStatus(s Status, format string, args ...interface{}) {
	line.LogStatus(s, fmt.Sprintf(format, args...))
}

func (line *noninteractiveLine) Failed(e error) {
	firstLine, _, _ := strings.Cut(e.Error(), "\n")
	line.LogfStatus(StatusFailure, "Failed: %s", firstLine)
}

func (line *noninteractiveLine) setStatus(s Status) {
	line.status = s
}

func (nl *noninteractiveLogger) Pause() ResumeFn { return func() {} }
