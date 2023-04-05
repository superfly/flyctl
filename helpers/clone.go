package helpers

import (
	"fmt"
	"reflect"

	"github.com/jinzhu/copier"
	"github.com/superfly/flyctl/internal/sentry"
)

// Clone clones a structure.
//   - this will panic if the structure is not serializable
//   - See DeepCopyFallible
func Clone[T any](v T) T {
	ret, err := CloneFallible(v)
	// Reference: https://github.com/jinzhu/copier/blob/20cee7e229707f8e3fd10f8ed21f3e6c08ca9463/errors.go
	if err != nil {
		typename := fmt.Sprintf("%T", v)
		sentry.CaptureException(fmt.Errorf("failed to clone '%s': %w", typename, err))
		panic(fmt.Sprintf("failed to deep-copy '%s'. this is a bug!\nerror: %v", typename, err))
	}
	return ret
}

func CloneFallible[T any](v T) (T, error) {
	reflectedValue := reflect.ValueOf(v)
	if reflectedValue.Kind() == reflect.Ptr {

		var nilT T

		vPtr := reflectedValue.Interface()
		cloned := reflect.New(reflect.Indirect(reflectedValue).Type())
		if cloned.Interface() == nil {
			return nilT, nil
		}
		err := copier.CopyWithOption(cloned.Interface(), vPtr, copier.Option{IgnoreEmpty: true, DeepCopy: true})
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
		err := copier.CopyWithOption(&cloned, v, copier.Option{IgnoreEmpty: true, DeepCopy: true})
		return cloned, err
	}
}
