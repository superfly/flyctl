//go:build !windows
// +build !windows

package ssh

// Windows consoles need a bit of magic to correctly handle ANSI escape
// sequences. Stub these out for non-Windows platforms.

func setupConsole() (uint32, uint32, uint32, error) {
	return 0, 0, 0, nil
}

func cleanupConsole(_currentStdin uint32, _currentStdout uint32, _currentStderr uint32) error {
	return nil
}
