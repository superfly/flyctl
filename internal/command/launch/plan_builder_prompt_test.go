//go:build !windows

package launch

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestNudgeTowardsDeployInteractive(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		wantErr error
	}{
		{name: "accept launching a new app", answer: "y\r"},
		{name: "interrupt", answer: "\x03", wantErr: terminal.InterruptErr},
		{name: "input closed", wantErr: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, output := interactiveNudgeContext(t, tc.answer)
			taken, free, err := nudgeTowardsDeploy(ctx, "existing-app")

			require.ErrorIs(t, err, tc.wantErr)
			assert.True(t, taken)
			assert.False(t, free, "an existing app must never be reported as free")
			assert.Contains(t, output.String(), "fly deploy")
			assert.Contains(t, output.String(), "Do you still want to launch a new app?")
		})
	}
}

func TestDetermineAppNameInteractive(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		wantErr error
	}{
		{name: "accept generates a new name", answer: "y\r"},
		{name: "interrupt stops name selection", answer: "\x03", wantErr: terminal.InterruptErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := interactiveNudgeContext(t, tc.answer, "--name", "existing-app")
			var lookups, checked []string
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
					lookups = append(lookups, name)

					return &flaps.App{Name: name}, nil
				},
				AppNameAvailableFunc: func(_ context.Context, name string) (bool, error) {
					checked = append(checked, name)

					return true, nil
				},
			})
			name, cause, err := determineAppName(ctx, nil, &appconfig.Config{}, "fly.toml")

			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, []string{"existing-app"}, lookups)
			if tc.wantErr != nil {
				assert.Empty(t, name)
				assert.Empty(t, checked, "an interrupted prompt must not check another name")

				return
			}
			assert.Regexp(t, `^existing-app-[a-z]+-[a-z]+-\d+$`, name)
			assert.Equal(t, "generated", cause)
			assert.Equal(t, []string{name}, checked, "check only the generated name, not the known existing app")
		})
	}
}

func TestNudgeTowardsDeployDecline(t *testing.T) {
	const helperEnv = "FLYCTL_TEST_NUDGE_DECLINE"
	if answer := os.Getenv(helperEnv); answer != "" {
		ctx, _ := interactiveNudgeContext(t, answer, "--name", "existing-app")
		// Make the prompt visible to the parent before nudgeTowardsDeploy
		// exits, so a successful child exit alone cannot satisfy the test.
		iostreams.FromContext(ctx).Out = &nudgePromptOutput{Writer: os.Stdout}
		_, _, _ = determineAppName(ctx, nil, &appconfig.Config{}, "fly.toml")
		t.Fatal("declining must exit before name selection continues")
	}

	for _, answer := range []string{"n\r", "\r"} {
		t.Run(answer, func(t *testing.T) {
			// Declining calls os.Exit(0), so exercise it in a subprocess.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNudgeTowardsDeployDecline$")
			cmd.Env = append(os.Environ(), helperEnv+"="+answer)
			output, err := cmd.CombinedOutput()

			require.NoError(t, err, "child output: %s", output)
			assert.Contains(t, string(output), "fly deploy")
			assert.Contains(t, string(output), "Do you still want to launch a new app?")
			assert.Contains(t, string(output), "No")
		})
	}
}

func interactiveNudgeContext(t *testing.T, answer string, args ...string) (context.Context, *bytes.Buffer) {
	t.Helper()
	ctx := newNudgeCtx(t, args...)
	streams := iostreams.FromContext(ctx)
	output := new(bytes.Buffer)
	// Survey asks for the terminal size and cursor position before reading
	// the answer. Supplying those reports exercises the real prompt without
	// requiring a terminal or changing the process's terminal settings.
	streams.In = &nudgePromptInput{Reader: strings.NewReader("\x1b[24;80R\x1b[1;1R" + answer)}
	streams.Out = &nudgePromptOutput{Writer: output}
	streams.SetStdinTTY(true)
	streams.SetStdoutTTY(true)

	return ctx, output
}

type nudgePromptInput struct{ io.Reader }

func (in *nudgePromptInput) Read(p []byte) (int, error) {
	// Prevent the cursor-report reader from buffering and discarding the
	// following report or answer after it reads the report's final R.
	if len(p) > 1 {
		p = p[:1]
	}

	return in.Reader.Read(p)
}

func (*nudgePromptInput) Close() error { return nil }
func (*nudgePromptInput) Fd() uintptr  { return ^uintptr(0) }

type nudgePromptOutput struct{ io.Writer }

func (*nudgePromptOutput) Fd() uintptr { return ^uintptr(0) }
