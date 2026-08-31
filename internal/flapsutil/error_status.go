package flapsutil

import "github.com/superfly/fly-go/flaps"

const VolumePlacementCapacityStatus = flaps.StatusCode("volume_placement_capacity")

// HasErrorStatusCode reports whether err or any error it wraps carries status.
// Error pools can join several Flaps errors, so errors.As alone is insufficient:
// it can stop at an earlier Flaps error with a different status.
func HasErrorStatusCode(err error, status flaps.StatusCode) bool {
	if statusErr, ok := err.(flaps.ErrorStatusCode); ok {
		if actual := statusErr.StatusCode(); actual != nil && *actual == status {
			return true
		}
	}

	switch err := err.(type) {
	case interface{ Unwrap() []error }:
		for _, inner := range err.Unwrap() {
			if HasErrorStatusCode(inner, status) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		return HasErrorStatusCode(err.Unwrap(), status)
	}

	return false
}
