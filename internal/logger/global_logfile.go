package logger

import (
	"bufio"

	"github.com/superfly/flyctl/internal/logger/logfile"
)

var globalLogFile = logFile{
	destroyed: true,
}

func InitLogFile() error {
	if !globalLogFile.destroyed {
		return errLogfileAlreadyInitialized
	}
	rawFile, err := logfile.CreateLogFile()
	globalLogFile = logFile{
		file:      rawFile,
		writer:    bufio.NewWriter(rawFile),
		destroyed: false,
	}
	return err
}

func CloseLogFile() error {
	if globalLogFile.destroyed {
		return errLogfileAlreadyClosed
	}
	defer func() {
		globalLogFile.writer = nil
		globalLogFile.file = nil
		globalLogFile.destroyed = true
	}()

	return globalLogFile.Close()
}
