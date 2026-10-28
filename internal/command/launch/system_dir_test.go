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

func TestCheckNotInSystemDir(t *testing.T) {
	for _, tc := range []struct {
		dir     string
		wantErr bool
	}{
		{dir: `C:\WINDOWS\system32`, wantErr: true},
		{dir: `C:\Windows\System32`, wantErr: true},
		{dir: `c:/windows/system32/`, wantErr: true},
		{dir: `\\?\C:\Windows\System32`, wantErr: true},
		{dir: `C:\Windows`, wantErr: true},
		{dir: `C:\Users\me\code\my-app`},
		{dir: `C:\WindowsApps\my-app`},
		{dir: `D:\Windows\system32`},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			err := checkNotInSystemDir(tc.dir, `C:\Windows`)

			if !tc.wantErr {
				assert.NoError(t, err)

				return
			}
			assert.EqualError(t, err, "fly launch can't run in "+tc.dir)
			assert.Contains(t, flyerr.GetErrorSuggestion(err), "cd into your project")
		})
	}

	assert.NoError(t, checkNotInSystemDir(`C:\Windows\System32`, ""), "no SystemRoot, nothing to check")
	assert.NoError(t, checkNotInSystemDir(`C:\Users\me\my-app`, `C:\`), "a drive root isn't a system folder to refuse")
}

// Any directory launch can't write to fails up front, before the app is
// created; one that doesn't exist is left for launch to report.
func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, checkWritable(dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file is removed")

	assert.NoError(t, checkWritable(filepath.Join(dir, "missing")))

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions don't stop this user writing")
	}
	readOnly := filepath.Join(dir, "read-only")
	require.NoError(t, os.Mkdir(readOnly, 0o500))
	t.Cleanup(func() { os.Chmod(readOnly, 0o700) })

	err = checkWritable(readOnly)
	assert.EqualError(t, err, "fly launch can't write to "+readOnly)
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "write access")
}
