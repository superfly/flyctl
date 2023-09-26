package statuslogger

import (
	"fmt"
	"strings"
	"time"
)

type interactiveLine struct {
	logger      *interactiveLogger
	lineNum     int
	buf         string
	status      Status
	lastChanged time.Time
}

func (line *interactiveLine) updateTimestamp() {
	line.lastChanged = time.Now()
}

func (line *interactiveLine) Log(s string) {
	line.logger.lock.Lock()
	defer line.logger.lock.Unlock()
	line.buf = s
	line.logger.lockedDraw()
	line.updateTimestamp()
}

func (line *interactiveLine) Logf(format string, args ...interface{}) {
	line.Log(fmt.Sprintf(format, args...))
}

func (line *interactiveLine) LogStatus(s Status, str string) {
	line.logger.lock.Lock()
	defer line.logger.lock.Unlock()
	line.status = s
	line.buf = str
	line.logger.lockedDraw()
	line.updateTimestamp()
}

func (line *interactiveLine) LogfStatus(s Status, format string, args ...interface{}) {
	line.LogStatus(s, fmt.Sprintf(format, args...))
}

func (line *interactiveLine) Failed(e error) {
	firstLine, _, _ := strings.Cut(e.Error(), "\n")
	line.LogfStatus(StatusFailure, "Failed: %s", firstLine)
}

func (line *interactiveLine) setStatus(s Status) {
	line.logger.lock.Lock()
	defer line.logger.lock.Unlock()
	line.status = s
	line.logger.lockedDraw()
	line.updateTimestamp()
}
