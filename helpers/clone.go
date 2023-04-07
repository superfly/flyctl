package helpers

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/superfly/flyctl/internal/sentry"
)

// Clone clones *public fields* in a structure.
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

func deepCopy(from any, into any) error {
	// For some reason, this does not properly DeepCopy.
	// The test TestClonePointer would fail using this library.
	// return copier.CopyWithOption(into, from, copier.Option{IgnoreEmpty: true, DeepCopy: true})

	// Unfortunately, this _does_ deep copy, but this only copies public fields.
	jsonStr, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonStr, into)
}

// CloneFallible clones *public fields* in a structure.
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
