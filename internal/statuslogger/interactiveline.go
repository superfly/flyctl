package statuslogger

import (
	"fmt"
	"strings"
)

type interactiveLine struct {
	logger  *interactiveLogger
	lineNum int
	buf     string
	status  Status
}

func (sl *interactiveLine) Log(s string) {
	sl.logger.lock.Lock()
	defer sl.logger.lock.Unlock()
	sl.buf = s
	sl.logger.lockedDraw()
}

func (sl *interactiveLine) Logf(format string, args ...interface{}) {
	sl.Log(fmt.Sprintf(format, args...))
}

func (sl *interactiveLine) LogStatus(s Status, str string) {
	sl.logger.lock.Lock()
	defer sl.logger.lock.Unlock()
	sl.status = s
	sl.buf = str
	sl.logger.lockedDraw()
}

func (sl *interactiveLine) LogfStatus(s Status, format string, args ...interface{}) {
	sl.LogStatus(s, fmt.Sprintf(format, args...))
}

func (sl *interactiveLine) Failed(e error) {
	firstLine, _, _ := strings.Cut(e.Error(), "\n")
	sl.LogfStatus(StatusFailure, "Failed: %s", firstLine)
}

func (sl *interactiveLine) setStatus(s Status) {
	sl.logger.lock.Lock()
	defer sl.logger.lock.Unlock()
	sl.status = s
	sl.logger.lockedDraw()
}
