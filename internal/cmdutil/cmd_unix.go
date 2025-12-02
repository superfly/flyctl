//go:build !windows

package cmdutil

import (
	"os/exec"
	"syscall"
)

func SetSysProcAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}
