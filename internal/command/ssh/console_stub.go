//go:build !windows

package ssh

func setupConsole() (func(), error) {
	return func() {}, nil
}
