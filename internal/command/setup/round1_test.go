package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/require"
)

func TestNonTTYLoggedOutSkipsBrowserLogin(t *testing.T) {
	ctx, home, flags, streams, out, errOut := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	streams.SetStdinTTY(false)
	require.NoError(t, flags.Set("yes", "true"))
	loginFn = func(context.Context) error {
		t.Error("non-TTY setup must not start browser login")
		return nil
	}

	require.NoError(t, runAgent(ctx))
	require.FileExists(t, filepath.Join(home, ".claude.json"))
	require.FileExists(t, filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
	require.Contains(t, out.String(), "fly auth login")
	require.NotContains(t, out.String(), "Sign-in failed")
	require.NotContains(t, out.String(), "Opening")
	require.NotContains(t, errOut.String(), "successfully logged in")
}

func TestEmbeddedLoggedOutStaysOneLinePerAgent(t *testing.T) {
	t.Setenv(embeddedEnv, "1")
	ctx, home, flags, _, out, errOut := harness(t, loggedOut())
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, flags.Set("yes", "true"))
	loginFn = func(context.Context) error {
		t.Error("embedded setup must not start browser login")
		return nil
	}

	require.NoError(t, runAgent(ctx))
	text := out.String()
	require.NotContains(t, text, "Fly Agent Setup")
	require.NotContains(t, text, "Restart")
	require.NotContains(t, text, "Sign-in failed")
	require.NotContains(t, text, "Opening")
	require.NotContains(t, text, "successfully logged in")
	require.NotContains(t, text, authSkipHint)
	require.Contains(t, errOut.String(), authSkipHint)
	require.NotContains(t, errOut.String(), "Sign-in failed")
	require.NotContains(t, errOut.String(), "Opening")
	require.Equal(t, []string{
		"Universal (.agents): skills installed, MCP skipped (no MCP configuration for this agent)",
		"Claude Code: skills installed, MCP configured (" + filepath.Join(home, ".claude.json") + ")",
	}, nonEmptyLines(text))
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestSkillsFailureExitsNonZeroEvenIfMCPSucceeds(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	t.Setenv("FLY_SKILLS_TARBALL", filepath.Join(home, "missing.tar.gz"))
	require.NoError(t, flags.Set("yes", "true"))

	err := runAgent(ctx)
	require.EqualError(t, err, "setup failed to install skills")
	require.FileExists(t, filepath.Join(home, ".claude.json"))
}

func TestSkipSkillsSucceedsWhenMCPIsWritten(t *testing.T) {
	ctx, home, flags, _, _, _ := harness(t)
	mkdirAgent(t, home, "claude-code")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("skip-skills", "true"))

	require.NoError(t, runAgent(ctx))
	require.FileExists(t, filepath.Join(home, ".claude.json"))
	_, err := os.Stat(filepath.Join(home, ".claude", "skills", "fly", "SKILL.md"))
	require.True(t, os.IsNotExist(err))
}

func TestConfigEnvVarsRelocateSkillsAndMCP(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		ctx, home, flags, streams, out, _ := harness(t)
		codex := filepath.Join(home, "custom-codex")
		require.NoError(t, os.MkdirAll(codex, 0o755))
		t.Setenv("CODEX_HOME", codex)
		streams.SetStdinTTY(true)
		require.NoError(t, flags.Set("yes", "true"))

		require.NoError(t, runAgent(ctx))
		require.FileExists(t, filepath.Join(codex, "config.toml"))
		require.FileExists(t, filepath.Join(codex, "skills", "fly", "SKILL.md"))
		_, err := os.Stat(filepath.Join(home, ".codex", "config.toml"))
		require.True(t, os.IsNotExist(err))
		require.Contains(t, out.String(), codex)
	})

	t.Run("claude", func(t *testing.T) {
		ctx, home, flags, _, _, _ := harness(t)
		dir := filepath.Join(home, "custom-claude")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		require.NoError(t, flags.Set("yes", "true"))

		require.NoError(t, runAgent(ctx))
		require.Equal(t, hostedMCPURL, flyObject(t, filepath.Join(dir, ".claude.json"), "mcpServers")["url"])
		require.FileExists(t, filepath.Join(dir, "skills", "fly", "SKILL.md"))
		_, err := os.Stat(filepath.Join(home, ".claude.json"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("copilot", func(t *testing.T) {
		ctx, home, flags, _, _, _ := harness(t)
		dir := filepath.Join(home, "custom-copilot")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		t.Setenv("COPILOT_HOME", dir)
		require.NoError(t, flags.Set("yes", "true"))

		require.NoError(t, runAgent(ctx))
		require.Equal(t, []any{"*"}, flyObject(t, filepath.Join(dir, "mcp-config.json"), "mcpServers")["tools"])
		require.FileExists(t, filepath.Join(dir, "skills", "fly", "SKILL.md"))
		_, err := os.Stat(filepath.Join(home, ".copilot", "mcp-config.json"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("explicit codex dir is created", func(t *testing.T) {
		ctx, home, flags, _, out, _ := harness(t)
		dest := filepath.Join(home, "new-codex")
		t.Setenv("CODEX_HOME", dest)
		require.NoError(t, flags.Set("yes", "true"))
		require.NoError(t, flags.Set("agent", "codex"))

		require.NoError(t, runAgent(ctx))
		require.FileExists(t, filepath.Join(dest, "config.toml"))
		require.FileExists(t, filepath.Join(dest, "skills", "fly", "SKILL.md"))
		require.Contains(t, out.String(), dest)
	})
}

func TestXDGConfigHomeMovesOpenCode(t *testing.T) {
	xdg := t.TempDir()
	ctx, home, flags, _, out, _ := harness(t)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "opencode"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755))
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	cfg := filepath.Join(xdg, "opencode", "opencode.json")
	require.Equal(t, hostedMCPURL, flyObject(t, cfg, "mcp")["url"])
	require.FileExists(t, filepath.Join(xdg, "opencode", "skills", "fly", "SKILL.md"))
	_, err := os.Stat(filepath.Join(home, ".config", "opencode", "opencode.json"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(home, ".config", "opencode", "skills", "fly", "SKILL.md"))
	require.True(t, os.IsNotExist(err))
	require.Contains(t, out.String(), cfg)
}

func TestSymlinkedConfigIsUpdatedAtTarget(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "real-claude.json")
	link := filepath.Join(home, ".claude.json")
	writeExisting(t, real, []byte(`{"theme":"dark"}`), 0o640)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
	target, err := os.Lstat(real)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o640), target.Mode().Perm())
	}
	body := readFile(t, real)
	require.Contains(t, string(body), hostedMCPURL)
	require.Contains(t, string(body), `"theme":"dark"`)
	require.Contains(t, out.Detail, real)
}

func TestDanglingSymlinkIsNotReplaced(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(home, ".claude.json")
	if err := os.Symlink(filepath.Join(home, "missing.json"), link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Contains(t, out.Err.Error(), "symlink")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
	_, err = os.Stat(link)
	require.Error(t, err)
}

func TestSymlinkedParentDirIsFollowed(t *testing.T) {
	home := t.TempDir()
	realCursor := filepath.Join(home, "real-cursor")
	require.NoError(t, os.MkdirAll(realCursor, 0o755))
	link := filepath.Join(home, ".cursor")
	if err := os.Symlink(realCursor, link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "cursor", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.FileExists(t, filepath.Join(realCursor, "mcp.json"))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
	require.Contains(t, out.Detail, filepath.Join(realCursor, "mcp.json"))
}

func TestHardLinkKeepsPreviousInode(t *testing.T) {
	// Atomic rename replaces one directory entry. A second hard link keeps
	// the previous inode and therefore the previous contents.
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	other := filepath.Join(home, "other-name.json")
	original := []byte(`{"theme":"dark"}`)
	writeExisting(t, path, original, 0o644)
	if err := os.Link(path, other); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, other))
	require.Contains(t, string(readFile(t, path)), hostedMCPURL)
}

func TestConfigureMCPWaitsForFileLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	path := filepath.Join(home, ".claude.json")
	lockPath, err := configLockPath(path)
	require.NoError(t, err)
	mu := flock.New(lockPath)
	require.NoError(t, mu.Lock())

	done := make(chan mcpOutcome, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- configureMCP(home, "claude-code", false, false)
	}()
	<-started
	select {
	case out := <-done:
		require.NoError(t, mu.Unlock())
		t.Fatalf("configure finished while the lock was held: %+v", out)
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, mu.Unlock())

	select {
	case out := <-done:
		require.Equal(t, statusConfigured, out.Status, out.Detail)
	case <-time.After(6 * time.Second):
		t.Fatal("configure did not finish after the lock was released")
	}
	require.Equal(t, hostedMCPURL, flyObject(t, path, "mcpServers")["url"])
	_, err = os.Lstat(path + ".lock")
	require.True(t, os.IsNotExist(err), "lock file must not sit beside the config")
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(filepath.Dir(lockPath))
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestShapeMismatchIsAConflictUntilForce(t *testing.T) {
	home := t.TempDir()

	t.Run("copilot tools", func(t *testing.T) {
		path := filepath.Join(home, ".copilot", "mcp-config.json")
		original := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://mcp.fly.io","headers":{"X-Test":"1"}}}}`)
		writeExisting(t, path, original, 0o644)
		out := configureMCP(home, "copilot", false, false)
		require.Equal(t, statusConflict, out.Status, out.Detail)
		require.Equal(t, original, readFile(t, path))

		out = configureMCP(home, "copilot", true, false)
		require.Equal(t, statusConfigured, out.Status, out.Detail)
		entry := flyObject(t, path, "mcpServers")
		require.Equal(t, "http", entry["type"])
		require.Equal(t, hostedMCPURL, entry["url"])
		require.Equal(t, []any{"*"}, entry["tools"])
		require.Equal(t, "1", entry["headers"].(map[string]any)["X-Test"])
		assertNoSecrets(t, readFile(t, path))
	})

	t.Run("claude type", func(t *testing.T) {
		path := filepath.Join(home, ".claude.json")
		original := []byte(`{"mcpServers":{"fly":{"type":"stdio","url":"https://mcp.fly.io","headers":{"X-Test":"1"}}}}`)
		writeExisting(t, path, original, 0o644)
		require.Equal(t, statusConflict, configureMCP(home, "claude-code", false, false).Status)
		require.Equal(t, original, readFile(t, path))

		require.Equal(t, statusConfigured, configureMCP(home, "claude-code", true, false).Status)
		entry := flyObject(t, path, "mcpServers")
		require.Equal(t, "http", entry["type"])
		require.Equal(t, hostedMCPURL, entry["url"])
		require.Equal(t, "1", entry["headers"].(map[string]any)["X-Test"])
	})

	t.Run("disabled and enabled", func(t *testing.T) {
		factory := filepath.Join(home, ".factory", "mcp.json")
		factoryBody := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://mcp.fly.io","disabled":true,"headers":{"X-Test":"1"}}}}`)
		writeExisting(t, factory, factoryBody, 0o644)
		require.Equal(t, statusConflict, configureMCP(home, "factory-droid", false, false).Status)
		require.Equal(t, statusConfigured, configureMCP(home, "factory-droid", true, false).Status)
		entry := flyObject(t, factory, "mcpServers")
		require.Equal(t, false, entry["disabled"])
		require.Equal(t, "1", entry["headers"].(map[string]any)["X-Test"])

		opencode := filepath.Join(home, ".config", "opencode", "opencode.json")
		opencodeBody := []byte(`{"mcp":{"fly":{"type":"remote","url":"https://mcp.fly.io","enabled":false,"headers":{"X-Test":"1"}}}}`)
		writeExisting(t, opencode, opencodeBody, 0o644)
		require.Equal(t, statusConflict, configureMCP(home, "opencode", false, false).Status)
		require.Equal(t, statusConfigured, configureMCP(home, "opencode", true, false).Status)
		entry = flyObject(t, opencode, "mcp")
		require.Equal(t, true, entry["enabled"])
		require.Equal(t, "1", entry["headers"].(map[string]any)["X-Test"])
	})

	t.Run("cursor extra type is already configured", func(t *testing.T) {
		path := filepath.Join(home, ".cursor", "mcp.json")
		original := []byte(`{"mcpServers":{"fly":{"url":"https://mcp.fly.io","type":"sse"}}}`)
		writeExisting(t, path, original, 0o644)
		out := configureMCP(home, "cursor", false, false)
		require.Equal(t, statusAlready, out.Status, out.Detail)
		require.Equal(t, original, readFile(t, path))
	})
}

func TestURLTrailingSlashIsAlreadyConfigured(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude.json")
	original := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://mcp.fly.io/"}}}`)
	writeExisting(t, claude, original, 0o644)
	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, claude))

	wrong := []byte(`{"mcpServers":{"fly":{"type":"http","url":"http://mcp.fly.io"}}}`)
	writeExisting(t, claude, wrong, 0o644)
	require.Equal(t, statusConflict, configureMCP(home, "claude-code", false, false).Status)
	require.Equal(t, wrong, readFile(t, claude))

	otherPath := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://mcp.fly.io/other"}}}`)
	writeExisting(t, claude, otherPath, 0o644)
	require.Equal(t, statusConflict, configureMCP(home, "claude-code", false, false).Status)

	codex := filepath.Join(home, ".codex", "config.toml")
	tomlBody := []byte("[mcp_servers.fly]\nurl = \"https://mcp.fly.io/\"\nnote = \"keep-me\"\n")
	writeExisting(t, codex, tomlBody, 0o644)
	out = configureMCP(home, "codex", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, tomlBody, readFile(t, codex))
}

func TestAntigravityBothURLKeysMustMatch(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	wrong := []byte(`{"mcpServers":{"fly":{"serverUrl":"https://mcp.fly.io","url":"https://example.com","headers":{"X-Test":"1"}}}}`)
	writeExisting(t, path, wrong, 0o644)
	require.Equal(t, statusConflict, configureMCP(home, "antigravity", false, false).Status)
	require.Equal(t, wrong, readFile(t, path))

	require.Equal(t, statusConfigured, configureMCP(home, "antigravity", true, false).Status)
	entry := flyObject(t, path, "mcpServers")
	require.Equal(t, hostedMCPURL, entry["serverUrl"])
	require.Equal(t, hostedMCPURL, entry["url"])
	require.Equal(t, "1", entry["headers"].(map[string]any)["X-Test"])

	both := []byte(`{"mcpServers":{"fly":{"serverUrl":"https://mcp.fly.io/","url":"https://mcp.fly.io"}}}`)
	writeExisting(t, path, both, 0o644)
	out := configureMCP(home, "antigravity", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, both, readFile(t, path))
}

func TestDuplicateFlyKeys(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	disagree := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://example.com"},"fly":{"type":"http","url":"https://mcp.fly.io"}}}`)
	writeExisting(t, path, disagree, 0o644)
	require.Equal(t, statusConflict, configureMCP(home, "claude-code", false, false).Status)
	require.Equal(t, disagree, readFile(t, path))

	require.Equal(t, statusConfigured, configureMCP(home, "claude-code", true, false).Status)
	next := string(readFile(t, path))
	require.NotContains(t, next, "example.com")
	require.Equal(t, 2, strings.Count(next, hostedMCPURL))

	agree := []byte(`{"mcpServers":{"fly":{"type":"http","url":"https://mcp.fly.io"},"fly":{"type":"http","url":"https://mcp.fly.io/"}}}`)
	writeExisting(t, path, agree, 0o644)
	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, agree, readFile(t, path))
}

func TestOpenCodeV2Servers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	original := []byte(`{"mcp":{"servers":{"other":{"type":"remote","url":"https://example.com"}}},"theme":"dark"}`)
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "opencode", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	root := jsonObject(t, readFile(t, path))
	require.Equal(t, "dark", root["theme"])
	mcp := root["mcp"].(map[string]any)
	_, legacy := mcp["fly"]
	require.False(t, legacy)
	servers := mcp["servers"].(map[string]any)
	require.Equal(t, "https://example.com", servers["other"].(map[string]any)["url"])
	fly := servers["fly"].(map[string]any)
	require.Equal(t, "remote", fly["type"])
	require.Equal(t, hostedMCPURL, fly["url"])
	_, hasEnabled := fly["enabled"]
	require.False(t, hasEnabled)
	_, hasOAuth := fly["oauth"]
	require.False(t, hasOAuth)

	wrong := []byte(`{"mcp":{"servers":{"fly":{"type":"local","url":"https://mcp.fly.io","headers":{"X-Test":"1"}}}},"theme":"dark"}`)
	writeExisting(t, path, wrong, 0o644)
	require.Equal(t, statusConflict, configureMCP(home, "opencode", false, false).Status)
	require.Equal(t, wrong, readFile(t, path))
	require.Equal(t, statusConfigured, configureMCP(home, "opencode", true, false).Status)
	root = jsonObject(t, readFile(t, path))
	fly = root["mcp"].(map[string]any)["servers"].(map[string]any)["fly"].(map[string]any)
	require.Equal(t, "remote", fly["type"])
	require.Equal(t, hostedMCPURL, fly["url"])
	require.Equal(t, "1", fly["headers"].(map[string]any)["X-Test"])
	_, hasEnabled = fly["enabled"]
	require.False(t, hasEnabled)
	require.Equal(t, "dark", root["theme"])

	broken := []byte(`{"mcp":{"servers":"nope"},"theme":"dark"}`)
	writeExisting(t, path, broken, 0o644)
	out = configureMCP(home, "opencode", true, false)
	require.Equal(t, statusFailed, out.Status)
	require.Contains(t, out.Err.Error(), "mcp.servers")
	require.Equal(t, broken, readFile(t, path))
}

func TestUTF8BOM(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	body := []byte(`{"theme":"dark"}`)
	writeExisting(t, path, append(append([]byte{}, utf8BOM...), body...), 0o644)

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.True(t, bytes.HasPrefix(next, utf8BOM))
	require.Contains(t, string(next), `"theme":"dark"`)
	require.Contains(t, string(next), hostedMCPURL)

	writeExisting(t, path, append(append([]byte{}, utf8BOM...), []byte(" \n")...), 0o644)
	out = configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.Equal(t, hostedMCPURL, flyObject(t, path, "mcpServers")["url"])
}

func jsonObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var root map[string]any
	require.NoError(t, json.Unmarshal(data, &root))

	return root
}
