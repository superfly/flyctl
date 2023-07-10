package logfile

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	timeSpec = time.RFC3339
	keepLogs = 10
)

func formatLogName(fileTime time.Time) string {
	return fmt.Sprintf("flyctl-%s.log", fileTime.Format(timeSpec))
}
func parseLogName(fileName string) (time.Time, error) {
	date := strings.TrimSuffix(strings.TrimPrefix(fileName, "flyctl-"), filepath.Ext(fileName))
	return time.Parse(timeSpec, date)
}
