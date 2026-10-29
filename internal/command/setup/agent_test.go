package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/command/skills"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func TestMain(m *testing.M) {
	loginFn = func(context.Context) error { return errors.New("login not stubbed") }
	signupFn = func(context.Context) error { return errors.New("signup not stubbed") }
	os.Exit(m.Run())
}

func TestDetectionMatchesSkills(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755))
	// A file is not an install. Codex stays undetected.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".codex"), []byte("nope"), 0o644))

	want, err := skills.ResolveTools(home, nil)
	require.NoError(t, err)
	got, err := skills.ResolveTools(home, nil)
	require.NoError(t, err)
	assert.Equal(t, slugs(want), slugs(got))
	assert.Equal(t, []string{"universal", "claude-code", "cursor", "pi"}, slugs(got))

	filtered, err := skills.ResolveTools(home, []string{"cursor", "claude-code", "cursor"})
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor", "claude-code"}, slugs(filtered))

	_, err = skills.ResolveTools(home, []string{"emacs"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestHelpRecordsEmbeddedContract(t *testing.T) {
	cmd := newAgent()
	assert.Contains(t, cmd.Long, "FLY_SETUP_EMBEDDED=1 fly setup agent -y")
	assert.Contains(t, cmd.Short, "Install Fly skills")
}

func TestOutputModes(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		home := homeWith(t, ".claude", ".cursor")
		out, err := runSetup(t, home, []string{"-y", "--skip-login", "--skip-skills"}, true)
		require.NoError(t, err)
		assert.Contains(t, out, "Fly Agent Setup")
		assert.Contains(t, out, "Setting up:")
		assert.Contains(t, out, "MCP configured")
		assert.Contains(t, out, hintRestart)
		assert.Contains(t, out, hintAuthorize)
		assert.Contains(t, out, "Skipping Fly login.")
	})

	t.Run("non-tty behaves as yes", func(t *testing.T) {
		home := homeWith(t, ".claude")
		out, err := runSetup(t, home, []string{"--skip-login", "--skip-skills"}, false)
		require.NoError(t, err)
		assert.Contains(t, out, "Fly Agent Setup")
		assert.Contains(t, out, "MCP configured")
		assert.Contains(t, out, hintRestart)
		assert.FileExists(t, filepath.Join(home, ".claude.json"))
	})

	t.Run("embedded", func(t *testing.T) {
		home := homeWith(t, ".claude", ".cursor")
		t.Setenv(embeddedEnv, "1")
		out, err := runSetup(t, home, []string{"-y", "--skip-login", "--skip-skills"}, false)
		require.NoError(t, err)
		assert.NotContains(t, out, "Fly Agent Setup")
		assert.NotContains(t, out, "Setting up:")
		assert.NotContains(t, out, hintRestart)
		assert.NotContains(t, out, hintAuthorize)
		assert.NotContains(t, out, "Summary")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		// universal, claude-code, cursor
		require.Len(t, lines, 3)
		assert.Contains(t, lines[1], "Claude Code:")
		assert.Contains(t, lines[1], "skills skipped")
		assert.Contains(t, lines[1], "MCP configured")
	})

	t.Run("json", func(t *testing.T) {
		home := homeWith(t, ".claude")
		out, err := runSetup(t, home, []string{"--json", "--skip-login", "--skip-skills"}, true)
		require.NoError(t, err)
		var report setupReport
		require.NoError(t, json.Unmarshal([]byte(out), &report))
		assert.Equal(t, []string{hintRestart, hintAuthorize}, report.Hints)
		assert.Equal(t, "skipped", report.Auth.Action)
		require.NotEmpty(t, report.Agents)
		var claude agentResult
		for _, agent := range report.Agents {
			if agent.Slug == "claude-code" {
				claude = agent
			}
		}
		assert.Equal(t, "configured", claude.MCP)
		assert.Equal(t, "skipped", claude.Skills)
		assert.NotContains(t, out, "Fly Agent Setup")
	})
}

func TestYesLoginFailureDoesNotBlockMCP(t *testing.T) {
	home := homeWith(t, ".claude")
	loginFn = func(context.Context) error { return errors.New("no browser in test") }
	t.Cleanup(func() {
		loginFn = func(context.Context) error { return errors.New("login not stubbed") }
	})

	out, err := runSetup(t, home, []string{"-y", "--skip-skills"}, false)
	require.NoError(t, err)
	assert.Contains(t, out, "no browser in test")
	assert.Contains(t, out, "fly auth login")
	assert.Contains(t, out, "MCP configured")
	assert.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestRequestedAgentFailureExitsNonZero(t *testing.T) {
	home := homeWith(t, ".claude")
	original := []byte("{not json\n")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), original, 0o644))

	_, err := runSetup(t, home, []string{"-y", "--skip-login", "--skip-skills", "--agent", "claude-code"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requested agent failed")
	got, readErr := os.ReadFile(filepath.Join(home, ".claude.json"))
	require.NoError(t, readErr)
	assert.Equal(t, original, got)
}

func TestPiSkillsOnlyInOutput(t *testing.T) {
	home := homeWith(t, filepath.Join(".pi", "agent"))
	out, err := runSetup(t, home, []string{"-y", "--skip-login", "--skip-skills"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing succeeded")
	assert.Contains(t, out, "Pi has no stable global MCP config")
	assert.NoFileExists(t, filepath.Join(home, ".pi", "agent", "mcp.json"))
}

func TestSetupInstallsSkillsAndMCPThenNoops(t *testing.T) {
	home := homeWith(t, ".claude", ".codex", ".cursor")
	tarball := filepath.Join(t.TempDir(), "skills.tar.gz")
	require.NoError(t, os.WriteFile(tarball, skillsTarball(t), 0o644))
	t.Setenv("FLY_SKILLS_TARBALL", tarball)
	t.Setenv("FLY_SKILLS_REF", "")

	out, err := runSetup(t, home, []string{"-y", "--skip-login"}, false)
	require.NoError(t, err)
	assert.Contains(t, out, "skills installed")
	assert.Contains(t, out, "MCP configured")

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "fly", "SKILL.md"),
		filepath.Join(".claude", "skills", "fly", "SKILL.md"),
		filepath.Join(".codex", "skills", "fly", "SKILL.md"),
		filepath.Join(".cursor", "skills", "fly", "SKILL.md"),
	} {
		body, readErr := os.ReadFile(filepath.Join(home, rel))
		require.NoError(t, readErr, rel)
		assert.Equal(t, "# Fly\n", string(body), rel)
	}
	manifest, err := os.ReadFile(filepath.Join(home, ".fly", "skills.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "SKILL.md")

	claudeBefore := readFile(t, filepath.Join(home, ".claude.json"))
	codexBefore := readFile(t, filepath.Join(home, ".codex", "config.toml"))
	cursorBefore := readFile(t, filepath.Join(home, ".cursor", "mcp.json"))
	assertFlyPointsAt(t, clientBySlug("claude-code"), claudeBefore, mcpURL)
	assert.Contains(t, string(codexBefore), `url = "https://mcp.fly.io"`)
	assertFlyPointsAt(t, clientBySlug("cursor"), cursorBefore, mcpURL)
	assertNoSecrets(t, claudeBefore)
	assertNoSecrets(t, codexBefore)
	assertNoSecrets(t, cursorBefore)

	again, err := runSetup(t, home, []string{"-y", "--skip-login"}, false)
	require.NoError(t, err)
	assert.Contains(t, again, "up to date")
	assert.Contains(t, again, "already configured")
	assert.Equal(t, claudeBefore, readFile(t, filepath.Join(home, ".claude.json")))
	assert.Equal(t, codexBefore, readFile(t, filepath.Join(home, ".codex", "config.toml")))
	assert.Equal(t, cursorBefore, readFile(t, filepath.Join(home, ".cursor", "mcp.json")))
}

func skillsTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := "# Fly\n"
	hdr := &tar.Header{Name: "skills-main/plugins/fly/skills/fly/SKILL.md", Mode: 0o644, Size: int64(len(body))}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

func homeWith(t *testing.T, dirs ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, dir := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(home, dir), 0o755))
	}

	return home
}

func runSetup(t *testing.T, home string, args []string, stdinTTY bool) (string, error) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cmd := newAgent()
	require.NoError(t, cmd.ParseFlags(args))

	var buf bytes.Buffer
	streams := &iostreams.IOStreams{In: io.NopCloser(&bytes.Buffer{}), Out: &buf, ErrOut: &buf}
	streams.SetStdinTTY(stdinTTY)
	streams.SetStdoutTTY(stdinTTY)

	ctx := iostreams.NewContext(context.Background(), streams)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = state.WithConfigDirectory(ctx, filepath.Join(home, ".fly"))
	ctx = state.WithWorkingDirectory(ctx, home)
	ctx = config.NewContext(ctx, &config.Config{})

	err := runAgent(ctx)

	return buf.String(), err
}

func slugs(tools []skills.CodingTool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Slug)
	}

	return out
}
