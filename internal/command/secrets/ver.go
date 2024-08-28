package secrets

import (
	"fmt"
	"regexp"
	"strconv"
)

var ErrVersNotFound = func(label string, ver Ver) error {
	return fmt.Errorf("%s(%s): not found", label, ver)
}

// Ver is a key version. It is representable in a byte.
// The value VerUnspec is treated as the lowest possible version
// number, and encodes as byte(255).
type Ver int

const VerUnspec Ver = -1

func (v Ver) asByte() uint8 {
	if v == VerUnspec {
		return 255
	}
	return byte(v)
}

func verFromByte(n byte) Ver {
	if n == 255 {
		return VerUnspec
	}
	return Ver(n)
}

func (v Ver) String() string {
	if v == VerUnspec {
		return "unspec"
	}
	return fmt.Sprintf("%d", byte(v))
}

func (v Ver) Incr() (Ver, error) {
	n := int(v)
	switch n {
	case 254:
		return VerUnspec, fmt.Errorf("cannot increment version beyond maximum")
	case 255:
		return Ver(0), nil
	default:
		return Ver(n + 1), nil
	}
}

func CompareVer(a, b Ver) int {
	return int(a) - int(b)
}

var labelVersionPat = regexp.MustCompile("^(.*)v([0-9]{1,3})$")

// splitLabelVersion splits a label into an integer version and the remaining label.
// It returns a version of VerUnspec if no label is present or if it would be out of range.
// Otherwise it returns a version from 0 to 254.
func splitLabelVersion(label string) (Ver, string) {
	m := labelVersionPat.FindSubmatch([]byte(label))
	if m == nil {
		return VerUnspec, label
	}

	l := string(m[1])
	nstr := string(m[2])
	n, _ := strconv.ParseInt(nstr, 10, 16)
	if n > 254 {
		return VerUnspec, label
	}

	return Ver(n), l
}

func joinLabelVersion(ver Ver, prefix string) string {
	return fmt.Sprintf("%sv%d", prefix, int(ver))
}
