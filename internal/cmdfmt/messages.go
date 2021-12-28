package cmdfmt

import (
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
)

// extract message printing from cmdctx until we find a better way to do this
// TODO: deprecate this package in favor of render.TextBlock
func PrintBegin(w io.Writer, args ...interface{}) {
	fmt.Fprintln(w, aurora.Green("==> "+fmt.Sprint(args...)))
}

func PrintDone(w io.Writer, args ...interface{}) {
	fmt.Fprintln(w, aurora.Gray(20, "--> "+fmt.Sprint(args...)))
}
