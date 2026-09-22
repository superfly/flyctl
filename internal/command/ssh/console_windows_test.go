package ssh

import (
	"os"
	"testing"

	"github.com/Azure/go-ansiterm/winterm"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestRedirectedWindowsHandles(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer file.Close()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	require.NoError(t, err)
	defer null.Close()
	for _, f := range []*os.File{file, reader, writer, null} {
		t.Run(f.Name(), func(t *testing.T) {
			_, err := winterm.GetConsoleMode(f.Fd())
			require.ErrorIs(t, err, windows.ERROR_INVALID_HANDLE)
			require.True(t, redirectedHandle(f.Fd(), err))
			cleanup, err := consoleModes([]uintptr{f.Fd()}, []uint32{winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING}, winterm.GetConsoleMode, winterm.SetConsoleMode, redirectedHandle)
			require.NoError(t, err)
			cleanup()
		})
	}
	require.False(t, redirectedHandle(uintptr(windows.InvalidHandle), windows.ERROR_INVALID_HANDLE))
	require.False(t, redirectedHandle(file.Fd(), windows.ERROR_ACCESS_DENIED))
}
