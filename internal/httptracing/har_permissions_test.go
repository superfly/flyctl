package httptracing

import (
	"os"
	"path/filepath"
	"testing"
)

// The HAR archive contains request headers, including Authorization bearer
// tokens, so it must not be readable by other users on shared machines.
func TestFinishWritesHARWithOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.har")
	t.Setenv("FLYCTL_OUTPUT_HAR", path)

	previous := har
	t.Cleanup(func() { har = previous })
	har = nil

	Init()
	if har == nil {
		t.Fatal("Init did not enable HAR tracing with FLYCTL_OUTPUT_HAR set")
	}

	Finish()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("reading HAR file: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("HAR file permissions = %o, want 600", perm)
	}
}
