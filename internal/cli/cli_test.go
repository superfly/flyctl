package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/prompt"

	"github.com/superfly/flyctl/iostreams"
)

func TestUsageRecoveryHint(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
		args    []string
		err     error
		want    bool
	}{
		{name: "missing command", missing: true},
		{name: "unknown command", args: []string{"unknown"}, want: true},
		{name: "headless prompt", err: fmt.Errorf("wrapped: %w", prompt.ErrNonInteractive), want: true},
		{name: "ordinary failure", err: errors.New("remote failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "fly"}
			cmd.AddCommand(&cobra.Command{Use: "deploy", Run: func(*cobra.Command, []string) {}})
			if tc.missing {
				cmd = nil
			}
			ios, _, stdout, stderr := iostreams.Test()
			printUsageHint(ios, cmd, tc.args, tc.err)
			require.Empty(t, stdout.String())
			if tc.want {
				require.Equal(t, "Run 'fly --help' for usage.\n", stderr.String())
			} else {
				require.Empty(t, stderr.String())
			}
		})
	}
}

func TestUnknownCommandExitAndStreams(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLY_NO_UPDATE_CHECK", "1")
	t.Setenv("FLY_NO_PROMPT", "1")
	ios, _, stdout, stderr := iostreams.Test()
	exitCode := Run(context.Background(), ios, "not-a-fly-command")
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "unknown command")
	require.Contains(t, stderr.String(), "--help' for usage.")
}

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
