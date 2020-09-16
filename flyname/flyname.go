package flyname

import (
	"os"
	"path/filepath"
)

var cachedName string

// Name - get the (cached) executable name
func Name() string {
	if cachedName == "" {
		execname, err := os.Executable()
		if err != nil {
			panic(err)
		}
		cachedName = filepath.Base(execname)
	}
	return cachedName
}
