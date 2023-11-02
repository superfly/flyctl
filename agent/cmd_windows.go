//go:build windows
// +build windows

package agent

import (
	"fmt"
	"os/exec"
	"os/user"
	"syscall"

	"golang.org/x/sys/windows"
)

func SetSysProcAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

// Use UNIX sockets since 10.0.17063
// https://devblogs.microsoft.com/commandline/af_unix-comes-to-windows/
func UseUnixSockets() bool {
	maj, _, patch := windows.RtlGetNtVersionNumbers()
	if maj > 10 || maj == 10 && patch >= 17063 {
		return true
	}

	return false
}

func PipeName() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("can't query current username: %w", err)
	}

	return `\\.\pipe\fly-agent-` + user.Username, nil
}
