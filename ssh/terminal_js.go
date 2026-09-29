//go:build js

package ssh

import (
	"context"
	"errors"

	"golang.org/x/crypto/ssh"
)

// There are no console handles in a browser; the page supplies the size
// through SessionIO.Window and SessionIO.Resizes instead.
func (s *SessionIO) getAndWatchSize(ctx context.Context, sess *ssh.Session) (int, int, error) {
	return 0, 0, errors.New("no console: set SessionIO.Window")
}
