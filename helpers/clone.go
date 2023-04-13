package helpers

import (
	"fmt"
	"reflect"

	"github.com/jinzhu/copier"
	"github.com/superfly/flyctl/internal/sentry"
)

// Clone clones *public fields* in a structure.
// Private fields may or may not be copied, and private pointers may or may not point into the original object.
//   - this will panic if the structure is not serializable
//   - See CloneFallible
func Clone[T any](v T) T {
	ret, err := CloneFallible(v)
	// Reference for what errors can be returned:
	// https://github.com/jinzhu/copier/blob/20cee7e229707f8e3fd10f8ed21f3e6c08ca9463/errors.go
	if err != nil {
		typename := fmt.Sprintf("%T", v)
		sentry.CaptureException(fmt.Errorf("failed to clone '%s': %w", typename, err))
		panic(fmt.Sprintf("failed to deep-copy '%s'. this is a bug!\nerror: %v", typename, err))
	}
	return ret
}

// deepCopy is a little helper so that the implementation can be easily swapped out
// If from is type T, into should be *T and non-nil
func deepCopy(from any, into any) error {
	return copier.CopyWithOption(into, from, copier.Option{IgnoreEmpty: true, DeepCopy: true})
}

// CloneFallible clones *public fields* in a structure.
// Private fields may or may not be copied, and private pointers may or may not point into the original object.
//   - returns an error if the structure is not serializable
//   - See Clone
func CloneFallible[T any](v T) (T, error) {
	reflectedValue := reflect.ValueOf(v)
	if reflectedValue.Kind() == reflect.Ptr {

		var nilT T

		if reflectedValue.IsNil() {
			return nilT, nil
		}

		cloned := reflect.New(reflect.Indirect(reflectedValue).Type())
		err := deepCopy(v, cloned.Interface())
		if err != nil {
			return nilT, err
		}
		ret, ok := cloned.Interface().(T)
		if !ok {
			return nilT, fmt.Errorf("could not convert pointer back to generic T, got type %v", cloned.Type())
		}

		return ret, nil
	} else {
		var cloned T
		err := deepCopy(v, &cloned)
		return cloned, err
	}
}
