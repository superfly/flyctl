//go:build windows

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
	"github.com/superfly/flyctl/agent"
)

func removeSocket(path string) (err error) {
	if !agent.UseUnixSockets() {
		return nil
	}

	switch _, err = os.Lstat(path); {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
		break
	default:
		if err = os.Remove(path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}

	return
}

func bindSocket(socket string) (net.Listener, error) {
	if agent.UseUnixSockets() {
		return bindUnixSocket(socket)
	}

	pipe, err := agent.PipeName()
	if err != nil {
		return nil, err
	}
	// Default Named Pipe security policy grants full control to the LocalSystem
	// account, administrators, and the creator owner. It also grants read
	// access to members of the Everyone group and the anonymous account.
	// Read allowed to everyone is Ok, because we need to write to pipe to actually
	// establish a connection.
	l, err := winio.ListenPipe(pipe, nil)
	if err != nil {
		err = fmt.Errorf("failed binding: %w", err)
	}

	return l, nil
}
