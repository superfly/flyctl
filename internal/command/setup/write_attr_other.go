//go:build !windows

package setup

// prepareReplace is a no-op off Windows. Readonly files there are expressed
// by permissions, which writeConfig already preserves.
func prepareReplace(string) (func(), error) {
	return func() {}, nil
}
