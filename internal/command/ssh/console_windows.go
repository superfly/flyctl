package ssh

import (
	"errors"
	"os"

	"github.com/Azure/go-ansiterm/winterm"
	"golang.org/x/sys/windows"
)

func setupConsole() (func(), error) {
	return consoleModes(
		[]uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		[]uint32{winterm.ENABLE_VIRTUAL_TERMINAL_INPUT, winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING, winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING},
		winterm.GetConsoleMode, winterm.SetConsoleMode, redirectedHandle,
	)
}

func redirectedHandle(handle uintptr, err error) bool {
	if !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		return false
	}
	kind, typeErr := windows.GetFileType(windows.Handle(handle))
	return typeErr == nil && (kind == windows.FILE_TYPE_DISK || kind == windows.FILE_TYPE_PIPE || kind == windows.FILE_TYPE_CHAR)
}
