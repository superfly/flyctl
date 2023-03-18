package ssh

import (
	"os"

	"github.com/Azure/go-ansiterm/winterm"
)

// Windows consoles need a bit of magic to correctly handle ANSI escape
// sequences.
func setupConsole() (uint32, uint32, uint32, error) {
	var (
		currentStdinMode  uint32
		currentStdoutMode uint32
		currentStderrMode uint32
	)

	stdinFd := os.Stdin.Fd()
	if currentStdinMode, err := winterm.GetConsoleMode(stdinFd); err == nil {
		err := winterm.SetConsoleMode(stdinFd, currentStdinMode|winterm.ENABLE_VIRTUAL_TERMINAL_INPUT)
		if err != nil {
			return 0, 0, 0, err
		}
	} else {
		return 0, 0, 0, err
	}

	stdoutFd := os.Stdout.Fd()
	if currentStdoutMode, err := winterm.GetConsoleMode(stdoutFd); err == nil {
		err := winterm.SetConsoleMode(stdoutFd, currentStdoutMode|winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		if err != nil {
			return 0, 0, 0, err
		}
	} else {
		return 0, 0, 0, err
	}

	stderrFd := os.Stderr.Fd()
	if currentStderrMode, err := winterm.GetConsoleMode(stderrFd); err == nil {
		err := winterm.SetConsoleMode(stderrFd, currentStderrMode|winterm.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		if err != nil {
			return 0, 0, 0, err
		}
	} else {
		return 0, 0, 0, err
	}

	return currentStdinMode, currentStdoutMode, currentStderrMode, nil
}

func cleanupConsole(stdinMode uint32, stdoutMode uint32, stderrMode uint32) error {
	stdinFd := os.Stdin.Fd()
	err := winterm.SetConsoleMode(stdinFd, stdinMode)
	if err != nil {
		return err
	}

	stdoutFd := os.Stdout.Fd()
	err = winterm.SetConsoleMode(stdoutFd, stdoutMode)
	if err != nil {
		return err
	}

	stderrFd := os.Stderr.Fd()
	err = winterm.SetConsoleMode(stderrFd, stderrMode)
	if err != nil {
		return err
	}

	return nil
}
