package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/superfly/flyctl/helpers"
)

func logDir() (string, error) {
	configDir, err := helpers.GetConfigDirectory()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(configDir, "logs")

	dirStat, err := os.Stat(dir)
	if err == nil && !dirStat.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	} else if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	return dir, nil
}

// retainLogs ensures that the number of logs in the logs directory is less than or equal to keepLogs.
func retainLogs(logsDir string) error {

	type logEntry struct {
		path string
		time time.Time
	}
	var matchingLogs []logEntry

	// Iterate through the log directory, and take note of all log files fitting the flyctl-*.log pattern.
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".log" && strings.HasPrefix(info.Name(), "flyctl-") {
			date, err := parseLogName(info.Name())
			if err != nil {
				return err
			}
			matchingLogs = append(matchingLogs, logEntry{
				path: path,
				time: date,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Sort logs by timestamp
	sort.Slice(matchingLogs, func(i, j int) bool {
		return matchingLogs[i].time.After(matchingLogs[j].time)
	})

	// Delete all but the most recent [keepLogs] logs
	var lastErr error
	for i := keepLogs; i < len(matchingLogs); i++ {
		if err := os.Remove(matchingLogs[i].path); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func CreateLogFile() (*os.File, error) {

	logsDir, err := logDir()
	if err != nil {
		return nil, err
	}

	fileName := formatLogName(time.Now())
	filePath := filepath.Join(logsDir, fileName)

	rawFile, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	if err := retainLogs(logsDir); err != nil {
		return nil, err
	}

	return rawFile, nil
}
