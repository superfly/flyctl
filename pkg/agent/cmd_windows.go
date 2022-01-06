//go:build windows
// +build windows

package agent

import (
	"os/exec"
	"syscall"
)

func setCommandFlags(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
