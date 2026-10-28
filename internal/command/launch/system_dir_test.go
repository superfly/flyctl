package launch

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
)

// Any directory launch can't write to fails up front, before the app is
// created, and inside the system folder the error says how people get there.
// A writable directory passes wherever it is (LocalSystem's CI workspaces
// live under C:\Windows), and one that doesn't exist is left for launch.
func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, checkWritable(dir, dir), "writable, even inside the system root")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file is removed")

	assert.NoError(t, checkWritable(filepath.Join(dir, "missing"), ""))

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions don't stop this user writing")
	}
	readOnly := filepath.Join(dir, "read-only")
	require.NoError(t, os.Mkdir(readOnly, 0o500))
	t.Cleanup(func() { os.Chmod(readOnly, 0o700) })

	err = checkWritable(readOnly, "")
	assert.EqualError(t, err, "fly launch can't write to "+readOnly)
	assert.NotContains(t, flyerr.GetErrorDescription(err), "Windows system folder")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "cd into your project")

	err = checkWritable(readOnly, dir)
	assert.Contains(t, flyerr.GetErrorDescription(err), "That's a Windows system folder")
}

func TestIsWithin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "Windows")

	assert.True(t, isWithin(root, root))
	assert.True(t, isWithin(filepath.Join(root, "System32"), root))
	assert.True(t, isWithin(filepath.Join(string(filepath.Separator), "WINDOWS", "system32"), root), "case-insensitive")
	assert.False(t, isWithin(filepath.Join(string(filepath.Separator), "WindowsApps"), root))
	assert.False(t, isWithin(root, ""))
}
