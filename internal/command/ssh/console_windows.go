//go:build windows
// +build windows

package ssh

import (
	"fmt"
	"os"

	"github.com/Azure/go-ansiterm/winterm"
)

func init() {
	stdoutFd := os.Stdout.Fd()
	if mode, err := winterm.GetConsoleMode(stdoutFd); err == nil {
		err := winterm.SetConsoleMode(stdoutFd, mode|winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to enable virtual terminal processing for stdout:", err)
			return
		}
	}

	stdinFd := os.Stdin.Fd()
	if mode, err := winterm.GetConsoleMode(stdinFd); err == nil {
		err := winterm.SetConsoleMode(stdinFd, mode|winterm.ENABLE_VIRTUAL_TERMINAL_INPUT)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to enable virtual terminal processing for stdin:", err)
		}
	}
}
