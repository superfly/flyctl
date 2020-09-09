package flyname

import (
	"os"
	"path"
)

var CachedName string

func Name() string {
	if CachedName == "" {
		execname, err := os.Executable()
		if err != nil {
			panic(err)
		}
		CachedName = path.Base(execname)
	}
	return CachedName
}
