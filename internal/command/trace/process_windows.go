package trace

import (
	"os"
	"os/exec"
)

func childSignalForwarder(child *exec.Cmd) func(os.Signal) {
	// Console interrupts are delivered to the child by Windows.
	return func(os.Signal) {}
}

func childExitCode(err *exec.ExitError) int { return err.ExitCode() }
