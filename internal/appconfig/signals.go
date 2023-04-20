package appconfig

import (
	"syscall"

	"github.com/superfly/flyctl/api"
)

var signalSyscallMap = map[string]*api.Signal{
	"SIGABRT": {Signal: syscall.SIGABRT},
	"SIGALRM": {Signal: syscall.SIGALRM},
	"SIGFPE":  {Signal: syscall.SIGFPE},
	"SIGILL":  {Signal: syscall.SIGILL},
	"SIGINT":  {Signal: syscall.SIGINT},
	"SIGKILL": {Signal: syscall.SIGKILL},
	"SIGPIPE": {Signal: syscall.SIGPIPE},
	"SIGQUIT": {Signal: syscall.SIGQUIT},
	"SIGSEGV": {Signal: syscall.SIGSEGV},
	"SIGTERM": {Signal: syscall.SIGTERM},
	"SIGTRAP": {Signal: syscall.SIGTRAP},
	"SIGUSR1": {Signal: syscall.Signal(0xa)}, // SIGUSR1 Doesn't exist on windows
}
