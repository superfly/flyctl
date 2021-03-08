//go:build windows
// +build windows

package ssh

func watchWindowSize(ctx context.Context, fd int, sess *ssh.Session) error {
	// TODO: SIGWINCH for windows?
	return nil
}
