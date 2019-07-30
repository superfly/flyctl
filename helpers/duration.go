package helpers

import (
	"math"
	"time"
)

func Duration(d time.Duration, dicimal int) time.Duration {
	shift := int(math.Pow10(dicimal))

	units := []time.Duration{time.Second, time.Millisecond, time.Microsecond, time.Nanosecond}
	for _, u := range units {
		if d > u {
			div := u / time.Duration(shift)
			if div == 0 {
				break
			}
			d = d / div * div
			break
		}
	}

	return d
}
