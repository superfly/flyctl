package cmdutil

import (
	"os"
	"path/filepath"
)

func WorkingDirFromArg(args []string, indexPosition int) (wd string, err error) {
	if wd, err = os.Getwd(); err != nil {
		return
	}
	if wd, err = filepath.Abs(wd); err != nil {
		return
	}
	if len(args) < indexPosition+1 {
		return
	}

	argWd := args[indexPosition]
	if argWd == "" {
		return
	}

	if filepath.IsAbs(argWd) {
		return argWd, nil
	}

	wd = filepath.Join(wd, argWd)
	return
}
