package prompt

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

func TestIsNonInteractive(t *testing.T) {
	cases := []struct {
		err error
		exp bool
	}{
		{assert.AnError, false},
		{fmt.Errorf("wrapped: %w", assert.AnError), false},
		{ErrNonInteractive, true},
		{fmt.Errorf("wrapped: %w", ErrNonInteractive), true},
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

// ConfirmOverwrite must not reach past the IOStreams to the process's own
// stdin and stdout: without a terminal that blocks on an open pipe, or writes
// the prompt into the command's output.
func TestConfirmOverwriteNonInteractive(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer devNull.Close()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()

	origIn, origOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = devNull, w
	defer func() { os.Stdin, os.Stdout = origIn, origOut }()

	ios, _, _, _ := iostreams.Test()
	confirm, err := ConfirmOverwrite(iostreams.NewContext(context.Background(), ios), "Dockerfile")

	w.Close()
	leaked, _ := io.ReadAll(r)

	assert.False(t, confirm)
	assert.ErrorIs(t, err, ErrNonInteractive)
	assert.Empty(t, string(leaked), "wrote to the process stdout")
}

// Never-prompt (FLY_NO_PROMPT) wins over streams that claim to be TTYs, as
// under an agent's pseudo-terminal that nobody answers.
func TestConfirmNeverPrompt(t *testing.T) {
	// Real files, so only never-prompt stands between Confirm and survey.
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	require.NoError(t, err)
	defer devNull.Close()

	ios := &iostreams.IOStreams{In: devNull, Out: devNull, ErrOut: devNull}
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetNeverPrompt(true)

	confirm, err := Confirm(iostreams.NewContext(context.Background(), ios), "Continue?")

	assert.False(t, confirm)
	assert.ErrorIs(t, err, ErrNonInteractive)
}
