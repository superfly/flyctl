// Package flyname implements an efficient way of resolving the name of the
// executable.
//
// Importing this package will result into a runtime panic in situations where
// os.Executable would report an error.
package flyname

import (
	"os"
	"path/filepath"
)

var cachedName string // populated during init

func init() {
	var err error
	if cachedName, err = os.Executable(); err != nil {
		panic(err)
	}
	cachedName = filepath.Base(cachedName)
}

// Name returns the name for the executable that started the current
// process.
//
// Name is safe for concurrent use.
func Name() string {
	return cachedName
}
