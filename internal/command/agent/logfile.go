package agent

import (
	"os"
	"strings"
	"sync"
)

// maxLogSize bounds the log the agent is currently writing to. The directory as
// a whole is pruned when a daemon starts, but that only reaps files nothing has
// touched for a day: the agent runs for days at a time and writes a single log
// for its whole lifetime, so its own log is never a candidate.
const maxLogSize = 128 << 20 // 128MB

// rotatingFile writes to path, moving the file aside once it has taken
// maxLogSize. Only the previous generation is kept, so the pair costs at most
// twice maxLogSize until the next daemon start prunes them.
type rotatingFile struct {
	path string

	mu      sync.Mutex
	file    *os.File
	written int64
}

func openRotatingFile(path string) (*rotatingFile, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	// Resume from the size on disk so restarting the agent against an existing
	// log doesn't hand it a fresh budget.
	var written int64
	if inf, err := file.Stat(); err == nil {
		written = inf.Size()
	}

	return &rotatingFile{path: path, file: file, written: written}, nil
}

func (rf *rotatingFile) Write(p []byte) (int, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.written+int64(len(p)) > maxLogSize {
		if err := rf.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := rf.file.Write(p)
	rf.written += int64(n)

	return n, err
}

// rotate counts the bytes it has written instead of measuring the file, so a
// log deleted from under the agent is replaced at the next threshold rather
// than growing forever on an unlinked descriptor.
func (rf *rotatingFile) rotate() error {
	// Windows refuses to rename an open file.
	_ = rf.file.Close()

	if err := os.Rename(rf.path, previousPath(rf.path)); err != nil && !os.IsNotExist(err) {
		return err
	}

	file, err := os.OpenFile(rf.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}

	rf.file = file
	rf.written = 0

	return nil
}

func (rf *rotatingFile) Sync() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	return rf.file.Sync()
}

func (rf *rotatingFile) Close() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	return rf.file.Close()
}

// previousPath keeps the .log suffix: fly doctor diag collects agent logs by
// that pattern.
func previousPath(path string) string {
	return strings.TrimSuffix(path, ".log") + ".prev.log"
}
