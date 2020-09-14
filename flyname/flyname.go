package flyname

import (
	"os"
	"path"
)

var cachedName string

// Name - get the (cached) executable name
func Name() string {
	if cachedName == "" {
		execname, err := os.Executable()
		if err != nil {
			panic(err)
		}
		cachedName = path.Base(execname)
	}
	return cachedName
}
