//go:build !windows

package sprites

import "syscall"

const addrInUseErr = syscall.EADDRINUSE
