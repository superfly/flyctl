package ssh

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsoleModes(t *testing.T) {
	broken := errors.New("console failure")
	for _, tc := range []struct {
		name           string
		getErr, setErr error
		redirect       bool
	}{
		{name: "all consoles"},
		{name: "redirected stdout", getErr: broken, redirect: true},
		{name: "read failure", getErr: broken},
		{name: "write failure", setErr: broken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := [3]uint32{0, 2, 4}
			var writes [3]int
			cleanup, err := consoleModes([]uintptr{0, 1, 2}, []uint32{8, 8, 8}, func(h uintptr) (uint32, error) {
				if h == 1 && tc.getErr != nil {
					return 0, tc.getErr
				}
				return current[h], nil
			}, func(h uintptr, m uint32) error {
				if h == 1 && tc.setErr != nil {
					return tc.setErr
				}
				writes[h]++
				current[h] = m

				return nil
			}, func(uintptr, error) bool { return tc.redirect })
			if (tc.getErr != nil && !tc.redirect) || tc.setErr != nil {
				require.Error(t, err)
				require.Nil(t, cleanup)
				require.Equal(t, 2, writes[0], "earlier console restored after partial setup")
				require.Zero(t, writes[2], "later console not modified")
			} else {
				require.NoError(t, err)
				require.Equal(t, uint32(12), current[2], "stderr configured even when stdout redirected")
				cleanup()
				if tc.redirect {
					require.Zero(t, writes[1], "redirected handle never changed or restored")
				}
			}
			require.Equal(t, [3]uint32{0, 2, 4}, current, "original modes including zero restored")
		})
	}
}
