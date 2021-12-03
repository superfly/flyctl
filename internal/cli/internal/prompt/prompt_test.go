package prompt

import (
	"fmt"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNonInteractive(t *testing.T) {
	cases := []struct {
		err error
		exp bool
	}{
		{assert.AnError, false},
		{fmt.Errorf("wrapped: %w", assert.AnError), false},
		{errNonInteractive, true},
		{fmt.Errorf("wrapped: %w", errNonInteractive), true},
		{NonInteractiveError("some error"), true},
	}

	for i, kase := range cases {
		assert.Equal(t, kase.exp, IsNonInteractive(kase.err), "case: %d", i)
	}
}

func TestNonInteractiveError(t *testing.T) {
	fn := func(exp string) bool {
		return NonInteractiveError(exp).Error() == exp
	}
	require.NoError(t, quick.Check(fn, nil))
}
