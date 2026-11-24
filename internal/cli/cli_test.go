package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
	term2 "github.com/superfly/flyctl/terminal"
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

func TestEnableDebugLogsFromArgs(t *testing.T) {
	orig := term2.DefaultLogger
	t.Cleanup(func() { term2.DefaultLogger = orig })
	t.Setenv("LOG_LEVEL", "info")

	ios, _, _, _ := iostreams.Test()
	Run(context.Background(), ios, "help")
	assert.Equal(t, logger.Info, term2.GetLogLevel())

	Run(context.Background(), ios, "help", "--debug")
	assert.Equal(t, logger.Debug, term2.GetLogLevel())
	assert.Equal(t, "debug", os.Getenv("LOG_LEVEL"))
}

func TestWrapParagraphs(t *testing.T) {
	in := "First paragraph that is long enough to need wrapping at twenty.\n\n" +
		"Second\nparagraph joins.\n\n" +
		"Steps:\n- one\n- two is a long item that wraps\n  - nested item that also wraps\n\n" +
		"    fly deploy --remote-only --ha=false\n"

	want := "First paragraph that\nis long enough to\nneed wrapping at\ntwenty.\n\n" +
		"Second paragraph\njoins.\n\n" +
		"Steps:\n- one\n- two is a long item\n  that wraps\n  - nested item that\n    also wraps\n\n" +
		"    fly deploy --remote-only --ha=false"

	if got := wrapParagraphs(in, 20); got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestUnknownGroupSubcommand(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	ios, _, stdout, stderr := iostreams.Test()
	code := Run(context.Background(), ios, "mpg", "lst")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, strings.ReplaceAll("Error: unknown command \"lst\" for \"fly mpg\"\n\nDid you mean this?\n\tlist\n\nRun 'fly mpg --help' for usage.\n\n", "fly", filepath.Base(executable)), stderr.String())
}
