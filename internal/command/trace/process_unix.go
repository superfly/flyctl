//go:build !windows

package trace

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/superfly/flyctl/internal/cmdutil"
	"golang.org/x/term"
)

func childSignalForwarder(child *exec.Cmd) func(os.Signal) {
	// Keep terminal jobs in the foreground group so the child can read stdin.
	// The terminal broadcasts interrupts to that group.
	if file, ok := child.Stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return func(sig os.Signal) {
			if sig != os.Interrupt {
				_ = child.Process.Signal(sig)
			}
		}
	}

	cmdutil.SetSysProcAttributes(child)
	return func(sig os.Signal) {
		_ = syscall.Kill(-child.Process.Pid, sig.(syscall.Signal))
	}
}

func childExitCode(err *exec.ExitError) int {
	status := err.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}
