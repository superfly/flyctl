package cli

import "golang.org/x/sys/windows"

func init() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")

	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	// Set console input codepage to UTF-8
	// https://learn.microsoft.com/en-us/windows/win32/intl/code-page-identifiers#:~:text=Unicode%20(UTF%2D7)-,65001,-utf%2D8
	setConsoleCP.Call(uintptr(65001))

	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	// Set console ouput codepage to UTF-8
	setConsoleOutputCP.Call(uintptr(65001))
}
