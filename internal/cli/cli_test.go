package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"

	"github.com/superfly/flyctl/iostreams"
)

func TestPrintErrorKeepsRecoveryDetailsOnStderr(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	err := fmt.Errorf("deploy failed: %w", flyerr.GenericErr{
		Err: "invalid configuration", Descript: "the service has no port",
		Suggest: "set internal_port", DocUrl: "https://docs.fly.io/reference/configuration",
	})
	printError(ios, ios.ColorScheme(), nil, err)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "deploy failed: invalid configuration")
	require.Contains(t, stderr.String(), "the service has no port")
	require.Contains(t, stderr.String(), "set internal_port")
	require.Contains(t, stderr.String(), "https://docs.fly.io/reference/configuration")
}

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
