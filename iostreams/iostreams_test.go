package iostreams

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeverPromptIsNotInteractive(t *testing.T) {
	ios, _, _, _ := Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	require.True(t, ios.IsInteractive())

	ios.SetNeverPrompt(true)

	assert.False(t, ios.IsInteractive())
	assert.True(t, ios.IsStdinTTY())
	assert.True(t, ios.IsStdoutTTY())
}

func TestCanPromptSubprocess(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		stdinTTY, stdoutTTY, off bool
		want                     bool
	}{
		{"terminal", true, true, false, true},
		{"stdout piped, as in fly launch | tee", true, false, false, true},
		{"stdin piped", false, true, false, false},
		{"FLY_NO_PROMPT", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			io, _, _, _ := Test()
			io.SetStdinTTY(tc.stdinTTY)
			io.SetStdoutTTY(tc.stdoutTTY)
			io.SetNeverPrompt(tc.off)

			assert.Equal(t, tc.want, io.CanPromptSubprocess())
		})
	}
}
