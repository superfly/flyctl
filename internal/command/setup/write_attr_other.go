//go:build !windows

package setup

import "os"

// prepareReplace is a no-op off Windows. Readonly files there are expressed
// by permissions, which writeConfig already preserves.
func prepareReplace(*os.Root, string) (func(), error) {
	return func() {}, nil
}
