package flag

import "github.com/spf13/pflag"

// displayNoOptDefValAnnotation holds the NoOptDefVal to show for a flag in
// help and generated docs, when it differs from the one pflag parses with.
const displayNoOptDefValAnnotation = "flyctl_display_no_opt_def_val"

// SetDisplayNoOptDefVal makes help and generated docs show display as f's
// NoOptDefVal (the `[="..."]` part of the usage line) instead of the value pflag
// actually parses with. Use it for flags whose NoOptDefVal is an internal
// sentinel. Anything that renders flag usage must go through
// WithDisplayNoOptDefVals for it to take effect.
func SetDisplayNoOptDefVal(f *pflag.Flag, display string) {
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[displayNoOptDefValAnnotation] = []string{display}
}

// WithDisplayNoOptDefVals runs render with every flag in fs that has a display
// NoOptDefVal (see SetDisplayNoOptDefVal) temporarily showing it, and restores
// the parsing value afterwards, even if render panics.
func WithDisplayNoOptDefVals(fs *pflag.FlagSet, render func()) {
	type saved struct {
		f   *pflag.Flag
		val string
	}
	var restore []saved
	defer func() {
		for _, s := range restore {
			s.f.NoOptDefVal = s.val
		}
	}()
	fs.VisitAll(func(f *pflag.Flag) {
		if d, ok := f.Annotations[displayNoOptDefValAnnotation]; ok && len(d) == 1 {
			restore = append(restore, saved{f, f.NoOptDefVal})
			f.NoOptDefVal = d[0]
		}
	})
	render()
}
