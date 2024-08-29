package secrets

// Taken from kmsfs/kms/ver.go.
// It is not essential that this file is kept in sync with that version,
// as long as the encoding properties dont change (mostly min/max/increment).

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
)

var ErrVersNotFound = func(label string, ver Ver) error {
	return fmt.Errorf("%s(%s): not found", label, ver)
}

// Ver is a key version. It is representable as binary using the binary Varint encoding.
// The value VerUnspec is encoded as a zero byte and is treated as the lowest possible version number.
// All other versions are offset by 1, ie. Ver(0) encodes as 1 in Varint format.
// Ver must be between VerUnspec and VerMax, and encodes as at most four bytes.
type Ver int32

const (
	VerUnspec Ver = -1
	VerZero   Ver = 0
	VerMax    Ver = 0x1000_0000 - 2 // 268_435_454, 9 digits.
)

func (v Ver) Append(buf []byte) ([]byte, error) {
	if !(VerUnspec <= v && v <= VerMax) {
		return nil, fmt.Errorf("ver out of range")
	}

	val := uint64(v + 1)
	return binary.AppendUvarint(buf, val), nil
}

func VerFromBuf(buf []byte) (Ver, []byte, error) {
	val, n := binary.Uvarint(buf[:4])
	if n <= 0 {
		return VerUnspec, nil, fmt.Errorf("error reading version: out of range value")
	}
	resid := buf[n:]

	if !(uint64(VerUnspec+1) <= val && val <= uint64(VerMax+1)) {
		return VerUnspec, nil, fmt.Errorf("error reading version: out of range value")
	}

	return Ver(val - 1), resid, nil
}

func (v Ver) String() string {
	if v == VerUnspec {
		return "unspec"
	}
	return fmt.Sprintf("%d", int(v))
}

func (v Ver) Incr() (Ver, error) {
	if v >= VerMax {
		return VerUnspec, fmt.Errorf("cannot increment version beyond maximum")
	}
	return v + 1, nil
}

func CompareVer(a, b Ver) int {
	return int(a) - int(b)
}

var labelVersionPat = regexp.MustCompile("^(.*)v([0-9]{1,9})$")

// splitLabelVersion splits a label into an integer version and the remaining label.
// It returns a version of VerUnspec if no label is present or if it would be out of range.
// Otherwise it returns a version from 0 to 254.
func splitLabelVersion(label string) (Ver, string) {
	m := labelVersionPat.FindStringSubmatch(label)
	if m == nil {
		return VerUnspec, label
	}

	l := string(m[1])
	nstr := string(m[2])
	n, _ := strconv.ParseUint(nstr, 10, 32)
	ver := Ver(n)
	if !(VerZero <= ver && ver <= VerMax) {
		return VerUnspec, label
	}

	return ver, l
}

func joinLabelVersion(ver Ver, prefix string) string {
	return fmt.Sprintf("%sv%d", prefix, int(ver))
}
