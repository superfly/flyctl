package ssh

import "fmt"

// consoleModes keeps restoration tied to handles whose modes we actually changed.
func consoleModes(handles []uintptr, modes []uint32, get func(uintptr) (uint32, error), set func(uintptr, uint32) error, redirected func(uintptr, error) bool) (func(), error) {
	var restore []func()
	cleanup := func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	}
	for i, handle := range handles {
		mode, err := get(handle)
		if err != nil {
			if redirected(handle, err) {
				continue
			}
			cleanup()
			return nil, fmt.Errorf("read console mode: %w", err)
		}
		if err := set(handle, mode|modes[i]); err != nil {
			cleanup()
			return nil, fmt.Errorf("set console mode: %w", err)
		}
		restore = append(restore, func() { _ = set(handle, mode) })
	}

	return cleanup, nil
}
