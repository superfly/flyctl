package cmdutil

import (
	"os"

	"github.com/mattn/go-isatty"
)

func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || IsCygwinTerminal(f)
}

func IsCygwinTerminal(f *os.File) bool {
	return isatty.IsCygwinTerminal(f.Fd())
}
