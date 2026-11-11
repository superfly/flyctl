package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/superfly/flyctl/iostreams"
)

func TestFlyNoPrompt(t *testing.T) {
	for val, interactive := range map[string]bool{"": true, "1": false} {
		t.Setenv("FLY_NO_PROMPT", val)

		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)
		setNeverPromptFromEnv(ios)

		assert.Equal(t, interactive, ios.IsInteractive(), "FLY_NO_PROMPT=%q", val)
	}
}

func TestWrapParagraphs(t *testing.T) {
	in := "First paragraph that is long enough to need wrapping at twenty.\n\n" +
		"Second\nparagraph joins.\n\n" +
		"Steps:\n- one\n- two is a long item that wraps\n\n" +
		"    fly deploy --remote-only\n"

	want := "First paragraph that\nis long enough to\nneed wrapping at\ntwenty.\n\n" +
		"Second paragraph\njoins.\n\n" +
		"Steps:\n- one\n- two is a long item\nthat wraps\n\n" +
		"    fly deploy\n    --remote-only"

	if got := wrapParagraphs(in, 20); got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}
