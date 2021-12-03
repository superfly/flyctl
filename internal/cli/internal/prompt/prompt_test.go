package prompt

import (
	"errors"
	"io"
	"testing"

	"github.com/tj/assert"
)

func TestNonInteractive(t *testing.T) {
	const exp = "some description"

	err := NonInteractiveError(exp)

	assert.False(t, errors.Is(err, io.EOF))
	assert.True(t, IsNonInteractive(err))
	assert.Equal(t, exp, err.Description())
	assert.Equal(t, "prompt: non-interactive", err.Error())
}
