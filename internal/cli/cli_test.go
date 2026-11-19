package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
