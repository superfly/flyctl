package flag

import (
	"fmt"
	"strings"
)

// regionValue implements pflag.Value for the region flag. A plain pflag string
// silently accepts a repeated flag (last value wins) and, through the `-r`
// shorthand, happily consumes an adjacent single-hyphen long flag such as
// `-restart=no` as if it were the region name `estart=no`. Both are almost
// always typos, so they are rejected with an explicit error.
type regionValue struct {
	flagName string
	value    string
	set      bool
}

func (v *regionValue) String() string {
	return v.value
}

func (v *regionValue) Set(value string) error {
	if v.set {
		return fmt.Errorf("flag --%s was set more than once; a command accepts only one region", v.flagName)
	}

	if strings.Contains(value, "=") {
		// A value containing '=' is almost certainly a long flag that lost one
		// of its leading hyphens (e.g. "-restart=no" reached this flag through
		// the "-r" shorthand).
		name := strings.TrimLeft(strings.SplitN(value, "=", 2)[0], "-")

		return fmt.Errorf("invalid region %q for flag --%s: did you mean --%s?", value, v.flagName, name)
	}

	v.value = value
	v.set = true

	return nil
}

func (v *regionValue) Type() string {
	return "string"
}
