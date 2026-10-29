package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/command/skills"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func TestHelpRecordsInstallerContract(t *testing.T) {
	long := newAgent().Long
	require.Contains(t, long, "FLY_SETUP_EMBEDDED=1 fly setup agent -y")
	require.Contains(t, long, "whitespace-only")
	require.Contains(t, long, "XDG_CONFIG_HOME")
	require.Contains(t, long, "CODEX_HOME")
	require.Contains(t, long, "CLAUDE_CONFIG_DIR")
	require.Contains(t, long, "COPILOT_HOME")
	require.Contains(t, long, "exits non-zero")
}

func TestYesSkipsPromptOnATTY(t *testing.T) {
	ctx, home, flags, streams, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	require.NoError(t, flags.Set("yes", "true"))
	confirmFn = func(context.Context, string) (bool, error) {
		t.Error("confirm should not be called with -y")
		return false, nil
	}

	require.NoError(t, runAgent(ctx))
	text := out.String()
	require.Contains(t, text, "Fly Agent Setup")
	require.Contains(t, text, "Signed in as dev@fly.io")
	require.Contains(t, text, restartHint)
	require.Contains(t, text, authorizeHint)
	require.Contains(t, text, "Claude Code: skills installed, MCP configured (")
	require.FileExists(t, filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
	require.FileExists(t, filepath.Join(home, ".fly", "skills.json"))
	require.Equal(t, hostedMCPURL, flyObject(t, filepath.Join(home, ".claude.json"), "mcpServers")["url"])
}

func TestNonTTYBehavesAsYes(t *testing.T) {
	ctx, home, _, streams, out, _ := harness(t)
	mkdirAgent(t, home, "cursor")
	streams.SetStdinTTY(false)
	confirmFn = func(context.Context, string) (bool, error) {
		t.Error("confirm should not be called when stdin is not a TTY")
		return false, nil
	}

	require.NoError(t, runAgent(ctx))
	require.Contains(t, out.String(), "Cursor: skills installed, MCP configured (")
	require.FileExists(t, filepath.Join(home, ".cursor", "mcp.json"))
}

func TestDeclineCancelsBeforeWriting(t *testing.T) {
	ctx, home, _, streams, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	confirmFn = func(context.Context, string) (bool, error) {
		return false, nil
	}

	err := runAgent(ctx)
	require.EqualError(t, err, "setup cancelled")
	_, statErr := os.Stat(filepath.Join(home, ".claude.json"))
	require.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
	require.True(t, os.IsNotExist(statErr))
}

func TestNonInteractiveConfirmProceeds(t *testing.T) {
	ctx, home, _, streams, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	confirmFn = func(context.Context, string) (bool, error) {
		return false, prompt.ErrNonInteractive
	}

	require.NoError(t, runAgent(ctx))
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestEmbeddedOutputIsOneLinePerAgent(t *testing.T) {
	t.Setenv(embeddedEnv, "1")
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code", "codex", "cursor")
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	text := out.String()
	require.NotContains(t, text, "Fly Agent Setup")
	require.NotContains(t, text, "Restart")
	require.NotContains(t, text, "restart")
	require.NotContains(t, text, authorizeHint)
	require.NotContains(t, text, "Signed in as")
	lines := nonEmptyLines(text)
	require.Equal(t, []string{
		"Universal (.agents): skills installed, MCP skipped (no MCP configuration for this agent)",
		"Claude Code: skills installed, MCP configured (" + filepath.Join(home, ".claude.json") + ")",
		"OpenAI Codex: skills installed, MCP configured (" + filepath.Join(home, ".codex", "config.toml") + ")",
		"Cursor: skills installed, MCP configured (" + filepath.Join(home, ".cursor", "mcp.json") + ")",
	}, lines)
}

func TestJSONSummary(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("json", "true"))

	require.NoError(t, runAgent(ctx))
	var rep report
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.True(t, rep.OK)
	require.Equal(t, "dev@fly.io", rep.Auth.Email)
	require.True(t, rep.Auth.OK)
	require.Equal(t, restartHint, rep.RestartHint)
	require.Equal(t, authorizeHint, rep.AuthorizeHint)
	require.NotContains(t, out.String(), "Fly Agent Setup")
	require.NotContains(t, out.String(), "Signed in as")

	var claude agentResult
	for _, agent := range rep.Agents {
		if agent.Slug == "claude-code" {
			claude = agent
		}
	}
	require.Equal(t, skillsInstalled, claude.Skills)
	require.Equal(t, statusConfigured, claude.MCP)
	require.Equal(t, filepath.Join(home, ".claude.json"), claude.MCPPath)
}

func TestEmbeddedJSONOmitsHints(t *testing.T) {
	t.Setenv(embeddedEnv, "1")
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "pi")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("json", "true"))

	require.NoError(t, runAgent(ctx))
	var rep report
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.Empty(t, rep.RestartHint)
	require.Empty(t, rep.AuthorizeHint)
	var pi agentResult
	for _, agent := range rep.Agents {
		if agent.Slug == "pi" {
			pi = agent
		}
	}
	require.Equal(t, "skipped (no stable global MCP config)", pi.MCPDetail)
	require.Equal(t, skillsInstalled, pi.Skills)
}

func TestLoginFailureDoesNotBlockSetup(t *testing.T) {
	ctx, home, flags, streams, out, _ := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	require.NoError(t, flags.Set("yes", "true"))
	loginFn = func(context.Context) error {
		// Writes finish before the browser login starts.
		require.FileExists(t, filepath.Join(home, ".claude.json"))
		require.FileExists(t, filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
		return errors.New("browser closed")
	}

	require.NoError(t, runAgent(ctx))
	require.Contains(t, out.String(), "Sign-in failed: browser closed")
	require.Contains(t, out.String(), "were not blocked")
	require.FileExists(t, filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestJSONLoginWritesToStderr(t *testing.T) {
	ctx, home, flags, streams, out, errOut := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("json", "true"))
	loginFn = func(ctx context.Context) error {
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "successfully logged in as dev@fly.io")
		return nil
	}

	require.NoError(t, runAgent(ctx))
	var rep report
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.NotContains(t, out.String(), "successfully logged in")
	require.Contains(t, errOut.String(), "successfully logged in as dev@fly.io")
	require.True(t, rep.OK)
}

func TestSkipLoginReminds(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("skip-login", "true"))

	require.NoError(t, runAgent(ctx))
	require.Contains(t, out.String(), "fly auth login")
	require.Contains(t, out.String(), "fly auth signup")
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestInteractiveSkipAndSignup(t *testing.T) {
	t.Run("skip", func(t *testing.T) {
		ctx, home, _, streams, out, _ := harness(t, loggedOut())
		mkdirAgent(t, home, "claude-code")
		streams.SetStdinTTY(true)
		confirmFn = func(context.Context, string) (bool, error) { return true, nil }
		selectFn = func(_ context.Context, index *int, _, _ string, _ ...string) error {
			*index = 2
			return nil
		}

		require.NoError(t, runAgent(ctx))
		require.Contains(t, out.String(), authSkipHint)
	})

	t.Run("signup", func(t *testing.T) {
		ctx, home, _, streams, _, _ := harness(t, loggedOut())
		mkdirAgent(t, home, "claude-code")
		streams.SetStdinTTY(true)
		confirmFn = func(context.Context, string) (bool, error) { return true, nil }
		signedUp := false
		signupFn = func(context.Context) error {
			signedUp = true
			return nil
		}
		selectFn = func(_ context.Context, index *int, _, _ string, _ ...string) error {
			*index = 1
			return nil
		}

		require.NoError(t, runAgent(ctx))
		require.True(t, signedUp)
	})
}

func TestUnparseableClientDoesNotFailOthers(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code", "cursor")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{"), 0o644))
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	require.Equal(t, []byte("{"), readFile(t, filepath.Join(home, ".claude.json")))
	require.Equal(t, hostedMCPURL, flyObject(t, filepath.Join(home, ".cursor", "mcp.json"), "mcpServers")["url"])
}

func TestRequestedAgentMCPFailureExitsNonZero(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{"), 0o644))
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("agent", "claude-code"))

	err := runAgent(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Claude Code")
	require.Equal(t, []byte("{"), readFile(t, filepath.Join(home, ".claude.json")))
	require.FileExists(t, filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
}

func TestConflictIsNotAFailure(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	original := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://example.com"}}}`)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), original, 0o644))
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	require.Equal(t, original, readFile(t, filepath.Join(home, ".claude.json")))
	require.Contains(t, out.String(), "points elsewhere")
}

func TestSecondRunLeavesMCPUntouched(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code", "codex", "cursor")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, runAgent(ctx))

	claude := readFile(t, filepath.Join(home, ".claude.json"))
	codex := readFile(t, filepath.Join(home, ".codex", "config.toml"))
	cursor := readFile(t, filepath.Join(home, ".cursor", "mcp.json"))
	out.Reset()

	require.NoError(t, runAgent(ctx))
	require.Equal(t, claude, readFile(t, filepath.Join(home, ".claude.json")))
	require.Equal(t, codex, readFile(t, filepath.Join(home, ".codex", "config.toml")))
	require.Equal(t, cursor, readFile(t, filepath.Join(home, ".cursor", "mcp.json")))
	require.Contains(t, out.String(), "already configured ("+filepath.Join(home, ".claude.json")+")")
}

func TestForceDoesNotOverwriteLocalSkillEdits(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, runAgent(ctx))

	skill := filepath.Join(home, ".claude", "skills", "fly", "SKILL.md")
	require.NoError(t, os.WriteFile(skill, []byte("# local edit\n"), 0o644))
	require.NoError(t, flags.Set("force", "true"))

	require.NoError(t, runAgent(ctx))
	require.Equal(t, "# local edit\n", string(readFile(t, skill)))
}

func TestDetectionMatchesSkills(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755))
	require.NoError(t, flags.Set("yes", "true"))

	want, err := skills.ResolveTools(home, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"universal", "claude-code", "opencode"}, slugsOf(want))

	require.NoError(t, runAgent(ctx))
	got, err := skills.ResolveTools(home, nil)
	require.NoError(t, err)
	require.Equal(t, slugsOf(want), slugsOf(got))

	require.FileExists(t, filepath.Join(home, ".claude.json"))
	require.FileExists(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	_, err = os.Stat(filepath.Join(home, ".cursor", "mcp.json"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(home, ".pi", "agent", "mcp.json"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(home, ".agents", "mcp.json"))
	require.True(t, os.IsNotExist(err))
}

func TestExplicitAgentIgnoresDetection(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("agent", "cursor"))

	_, err := os.Stat(filepath.Join(home, ".cursor"))
	require.True(t, os.IsNotExist(err))
	require.NoError(t, runAgent(ctx))
	require.Equal(t, hostedMCPURL, flyObject(t, filepath.Join(home, ".cursor", "mcp.json"), "mcpServers")["url"])
	_, err = os.Stat(filepath.Join(home, ".claude.json"))
	require.True(t, os.IsNotExist(err))
}

func TestSkillsFailureOfRequestedAgentExitsNonZero(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	t.Setenv("FLY_SKILLS_TARBALL", filepath.Join(home, "missing.tar.gz"))
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("agent", "claude-code"))

	err := runAgent(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Claude Code")
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestSkippingEverythingIsRefusedBeforeLogin(t *testing.T) {
	ctx, _, flags, streams, _, _ := harness(t, loggedOut())
	streams.SetStdinTTY(true)
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("skip-skills", "true"))
	require.NoError(t, flags.Set("skip-mcp", "true"))
	loginFn = func(context.Context) error {
		t.Fatal("login must not run when there is nothing to set up")
		return nil
	}

	err := runAgent(ctx)
	require.EqualError(t, err, "nothing to do: --skip-skills and --skip-mcp were both given")
}

func TestNothingSucceeded(t *testing.T) {
	// Skills skipped, and the only MCP target cannot be written.
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "cursor")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cursor", "mcp.json"), []byte("{ not json"), 0o644))
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("skip-skills", "true"))

	err := runAgent(ctx)
	require.EqualError(t, err, "setup did not install skills or configure MCP for any agent")
}

func TestHomeBehindSymlinkIsAccepted(t *testing.T) {
	// A HOME whose canonical path differs from its lexical path (macOS
	// /var -> /private/var, Fedora /home -> /var/home, Windows short names)
	// must not look like an escape from the config directory.
	ctx, realHome, flags, _, out, _ := harness(t)
	linkParent := t.TempDir()
	linkedHome := filepath.Join(linkParent, "home")
	require.NoError(t, os.Symlink(realHome, linkedHome))
	t.Setenv("HOME", linkedHome)
	t.Setenv("USERPROFILE", linkedHome)
	mkdirAgent(t, realHome, "claude-code", "cursor", "codex")
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	for _, rel := range []string{".claude.json", filepath.Join(".cursor", "mcp.json"), filepath.Join(".codex", "config.toml")} {
		require.FileExists(t, filepath.Join(realHome, rel), rel)
	}
	require.NotContains(t, out.String(), "outside the expected config directory")
	require.Contains(t, out.String(), "MCP configured")
}

func TestUnverifiableSignInIsNotAFailure(t *testing.T) {
	client := &mock.Client{
		AuthenticatedFunc: func() bool { return true },
		GetCurrentUserFunc: func(context.Context) (*fly.User, error) {
			return nil, errors.New("dial tcp: i/o timeout")
		},
	}
	ctx, home, flags, streams, out, _ := harness(t, client)
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	require.NoError(t, flags.Set("yes", "true"))
	loginFn = func(context.Context) error {
		t.Fatal("login must not run when the user may already be signed in")
		return nil
	}

	require.NoError(t, runAgent(ctx))
	require.Contains(t, out.String(), "Could not verify sign-in: dial tcp: i/o timeout")
	require.NotContains(t, out.String(), "Sign-in failed")
}

func TestPipedStdoutDoesNotOpenBrowserLogin(t *testing.T) {
	// stdin is a terminal but stdout is not, so the prompt cannot be shown.
	// Without -y the login must be skipped rather than launched unasked.
	ctx, home, _, streams, out, _ := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(true)
	confirmFn = func(context.Context, string) (bool, error) {
		return false, prompt.NonInteractiveError("no terminal")
	}
	selectFn = func(context.Context, *int, string, string, ...string) error {
		return prompt.NonInteractiveError("no terminal")
	}
	loginFn = func(context.Context) error {
		t.Fatal("login must not run without a prompt or -y")
		return nil
	}

	require.NoError(t, runAgent(ctx))
	require.Contains(t, out.String(), authSkipHint)
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestUnknownAgent(t *testing.T) {
	ctx, _, flags, _, _, _ := harness(t)
	require.NoError(t, flags.Set("agent", "nope"))

	err := runAgent(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown agent")
}

func harness(t *testing.T, client ...flyutil.Client) (context.Context, string, *pflag.FlagSet, *iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	silenceHooks(t)
	if value, ok := os.LookupEnv(embeddedEnv); ok && value == "" {
		// An empty value is still set, which is the embedded contract.
		// Tests opt in with t.Setenv. Clear a leaked empty value.
		require.NoError(t, os.Unsetenv(embeddedEnv))
		t.Cleanup(func() { _ = os.Setenv(embeddedEnv, value) })
	}

	// Canonicalize the temp dir: macOS puts it under /var -> /private/var and
	// Windows hands out 8.3 short names, and the command compares resolved paths.
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "COPILOT_HOME", "XDG_CONFIG_HOME"} {
		clearSetupEnv(t, key)
	}
	// ReadAccessToken locks flyctl.ConfigDir(), which is a process global.
	// Point it at the temp home so a logged-out run does not drop a lock
	// file in the package directory.
	flyctl.InitConfig()
	t.Setenv("FLY_SKILLS_REF", "")
	tarball := filepath.Join(home, "skills.tar.gz")
	require.NoError(t, os.WriteFile(tarball, skillsTarball(t), 0o644))
	t.Setenv("FLY_SKILLS_TARBALL", tarball)

	var out, errOut bytes.Buffer
	streams := &iostreams.IOStreams{Out: &out, ErrOut: &errOut}
	ctx := iostreams.NewContext(context.Background(), streams)
	ctx = state.WithConfigDirectory(ctx, filepath.Join(home, ".fly"))

	chosen := flyutil.Client(signedInClient())
	if len(client) > 0 {
		chosen = client[0]
	}
	ctx = flyutil.NewContextWithClient(ctx, chosen)

	flags := pflag.NewFlagSet("setup-test", pflag.ContinueOnError)
	flags.StringSlice("agent", nil, "")
	flags.Bool("yes", false, "")
	flags.Bool("force", false, "")
	flags.Bool("skip-mcp", false, "")
	flags.Bool("skip-skills", false, "")
	flags.Bool("skip-login", false, "")
	flags.Bool("json", false, "")
	ctx = flag.NewContext(ctx, flags)

	return ctx, home, flags, streams, &out, &errOut
}

func clearSetupEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() { _ = os.Setenv(key, value) })
}

func silenceHooks(t *testing.T) {
	t.Helper()
	prevConfirm, prevLogin, prevSignup, prevSelect := confirmFn, loginFn, signupFn, selectFn
	t.Cleanup(func() {
		confirmFn, loginFn, signupFn, selectFn = prevConfirm, prevLogin, prevSignup, prevSelect
	})
	confirmFn = func(context.Context, string) (bool, error) {
		t.Error("unexpected confirmation prompt")
		return false, nil
	}
	loginFn = func(context.Context) error {
		t.Error("unexpected login")
		return nil
	}
	signupFn = func(context.Context) error {
		t.Error("unexpected signup")
		return nil
	}
	selectFn = func(context.Context, *int, string, string, ...string) error {
		t.Error("unexpected select")
		return nil
	}
}

func signedInClient() *mock.Client {
	return &mock.Client{
		AuthenticatedFunc: func() bool { return true },
		GetCurrentUserFunc: func(context.Context) (*fly.User, error) {
			return &fly.User{Email: "dev@fly.io"}, nil
		},
	}
}

func loggedOut() *mock.Client {
	return &mock.Client{
		AuthenticatedFunc: func() bool { return false },
	}
}

func mkdirAgent(t *testing.T, home string, slugs ...string) {
	t.Helper()
	tools, err := skills.ResolveTools(home, slugs)
	require.NoError(t, err)
	for _, tool := range tools {
		require.NoError(t, os.MkdirAll(tool.Parent, 0o755))
	}
}

func slugsOf(tools []skills.CodingTool) []string {
	slugs := make([]string, 0, len(tools))
	for _, tool := range tools {
		slugs = append(slugs, tool.Slug)
	}

	return slugs
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func skillsTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("# Fly\n")
	hdr := &tar.Header{
		Name:     "skills-main/plugins/fly/skills/fly/SKILL.md",
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}
