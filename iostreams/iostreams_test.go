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
