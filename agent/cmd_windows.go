//go:build windows
// +build windows

package agent

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func setSysProcAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
