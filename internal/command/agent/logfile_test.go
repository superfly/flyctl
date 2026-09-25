package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// write enough lines to cross the threshold a few times over.
func writeLines(t *testing.T, rf *rotatingFile, line string, count int) {
	t.Helper()

	for range count {
		n, err := rf.Write([]byte(line))
		require.NoError(t, err)
		require.Equal(t, len(line), n)
	}
}

func TestRotatingFileBoundsTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	rf, err := openRotatingFile(path)
	require.NoError(t, err)
	defer rf.Close()

	line := strings.Repeat("x", 4096) + "\n"
	writeLines(t, rf, line, 3*(maxLogSize/len(line)))

	inf, err := os.Stat(path)
	require.NoError(t, err)
	require.LessOrEqual(t, inf.Size(), int64(maxLogSize))

	prev, err := os.Stat(previousPath(path))
	require.NoError(t, err, "the previous generation should be kept")
	require.LessOrEqual(t, prev.Size(), int64(maxLogSize))
}

func TestRotatingFileResumesFromSizeOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	require.NoError(t, os.WriteFile(path, make([]byte, maxLogSize), 0o600))

	rf, err := openRotatingFile(path)
	require.NoError(t, err)
	defer rf.Close()

	writeLines(t, rf, "hello\n", 1)

	prev, err := os.Stat(previousPath(path))
	require.NoError(t, err)
	require.Equal(t, int64(maxLogSize), prev.Size())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(got))
}

// The agent kept growing an unlinked descriptor after a user cleared the log
// directory by hand.
func TestRotatingFileReplacesADeletedLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	rf, err := openRotatingFile(path)
	require.NoError(t, err)
	defer rf.Close()

	line := strings.Repeat("x", 4096) + "\n"
	writeLines(t, rf, line, maxLogSize/len(line)/2)
	require.NoError(t, os.Remove(path))

	writeLines(t, rf, line, maxLogSize/len(line))

	inf, err := os.Stat(path)
	require.NoError(t, err, "writes should land in a fresh log")
	require.LessOrEqual(t, inf.Size(), int64(maxLogSize))
}

func TestPreviousPathKeepsTheLogSuffix(t *testing.T) {
	require.Equal(t, "/tmp/agent-logs/123.prev.log", previousPath("/tmp/agent-logs/123.log"))
}
